package middleware

import (
	"net/http"
	"sync"
	"time"

	"fogg-coach/config"
	"fogg-coach/db"

	"github.com/gin-gonic/gin"
)

// 进程内高频限流 + 日配额（system-design §3.5，行为对照 legacy/quota.go 移植）

type abuseState struct {
	mu        sync.Mutex
	buckets   map[string][]int64 // uid -> 滑动窗口时间戳
	cooldowns map[string]int64   // uid -> 冷却截止
	streaks   map[string]int     // uid -> 满长消息连击次数
}

var abuse = &abuseState{buckets: map[string][]int64{}, cooldowns: map[string]int64{}, streaks: map[string]int{}}

// RateLimit 每用户滑动窗口（>N msg/s → 冷却 30s），挂在 Auth 之后
func RateLimit() gin.HandlerFunc {
	go func() { // 周期清理（对照 legacy 60s 清理）
		for {
			time.Sleep(time.Minute)
			cutoff := time.Now().Add(-time.Minute).UnixMilli()
			abuse.mu.Lock()
			for k, arr := range abuse.buckets {
				var kept []int64
				for _, t := range arr {
					if t > cutoff {
						kept = append(kept, t)
					}
				}
				if len(kept) == 0 {
					delete(abuse.buckets, k)
				} else {
					abuse.buckets[k] = kept
				}
			}
			abuse.mu.Unlock()
		}
	}()
	return func(c *gin.Context) {
		u := CurrentUser(c)
		if u == nil {
			c.Next()
			return
		}
		cfg := config.Get()
		now := time.Now().UnixMilli()
		abuse.mu.Lock()
		defer abuse.mu.Unlock()
		if until := abuse.cooldowns[u.ID]; until > now {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "rate_limited", "message": "慢一点，30 秒后再试"})
			return
		}
		arr := abuse.buckets[u.ID]
		var kept []int64
		for _, t := range arr {
			if t > now-1000 {
				kept = append(kept, t)
			}
		}
		kept = append(kept, now)
		abuse.buckets[u.ID] = kept
		if len(kept) > cfg.Abuse.MsgsPerSec {
			abuse.cooldowns[u.ID] = now + int64(cfg.Abuse.CooldownSec)*1000
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "rate_limited", "message": "消息太快啦，休息 30 秒"})
			return
		}
		c.Next()
	}
}

// CheckDailyQuota 日配额 + 全站成本硬顶（chat/plan 路由内部调用）
// 返回 false 时已写好响应
func CheckDailyQuota(c *gin.Context) bool {
	cfg := config.Get()
	u := CurrentUser(c)
	day := db.Today()
	usage := db.UsageGet(u.ID, day)
	if usage.LLMMessages >= cfg.Quota.DailyMessages {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "quota", "message": "今天聊得够多啦，明天见 🌙"})
		return false
	}
	if cfg.GlobalCostCap > 0 && db.UsageTotalCost(day) >= cfg.GlobalCostCap {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "global_cap", "message": "教练今天休息了，明天再来"})
		return false
	}
	return true
}

// IsLongMsgStreak 满长消息连击检测（≥N 次疑似脚本 → 前端降级仅按钮模式）
func IsLongMsgStreak(userID string, msgLen int) bool {
	cfg := config.Get()
	if msgLen < cfg.Quota.MessageMaxLen*9/10 {
		delete(abuse.streaks, userID)
		return false
	}
	abuse.mu.Lock()
	defer abuse.mu.Unlock()
	abuse.streaks[userID]++
	return abuse.streaks[userID] >= cfg.Abuse.LongMsgStreak
}
