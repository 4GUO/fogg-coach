package routes

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"fogg-coach/config"
	"fogg-coach/db"
	"fogg-coach/fsm"
	"fogg-coach/llm"
	"fogg-coach/middleware"

	"github.com/gin-gonic/gin"
)

// POST /api/plan/generate — S7 结构化输出（§4.2）
// 409 阶段未到 | 429 配额 | 422 两次校验失败（回 S6）| 200 计划入库 → active

type planReq struct {
	SessionID string `json:"sessionId" binding:"required"`
}

type progRule struct {
	AfterCheckins any `json:"after_checkins"`
	Behavior      string `json:"behavior"`
}

type habitRule struct {
	ID         string     `json:"id"`
	Title      string     `json:"title"`
	Anchor     string     `json:"anchor"`
	Behavior   string     `json:"behavior"`
	Celebration string    `json:"celebration"`
	AnchorTime string     `json:"anchor_time"`
	Progression []progRule `json:"progression"`
}

type planDoc struct {
	Wish      string      `json:"wish"`
	Habits    []habitRule `json:"habits"`
	ReviewDay string      `json:"review_day"`
	CoachNote string      `json:"coach_note"`
}

var fenceRe = regexp.MustCompile("(?s)^```(?:json)?\\s*|\\s*```$")

func GeneratePlan(c *gin.Context) {
	user := middleware.CurrentUser(c)
	cfg := config.Get()
	var req planReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "缺少 sessionId"})
		return
	}
	sess, err := db.SessionByID(req.SessionID)
	if err == db.ErrNotFound {
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found", "message": "会话不存在"})
		return
	}
	if sess.Stage != string(fsm.S7) {
		c.JSON(http.StatusConflict, gin.H{"error": "stage_invalid", "message": "当前阶段不能生成计划（需 S6 确认后）"})
		return
	}
	usage := db.UsageGet(user.ID, db.Today())
	if usage.PlanGenerations >= cfg.Quota.PlanGenerations {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "quota", "message": "今天生成次数用完啦，明天再来"})
		return
	}

	ctx := fsm.ParseContext(sess.Context)
	result, err := provider.Chat(c.Request.Context(), llm.ChatOpts{
		Messages: llm.BuildPlanPrompt(ctx), JSONMode: true,
		Temperature: 0.3, Thinking: true, MaxTokens: 4000, // §3.2.5：S7 开思考 + 4000 防腰斩
	})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "llm_error", "message": "生成失败，请重试"})
		return
	}

	doc, reason := validatePlan(result.Content, ctx)
	if doc == nil { // 失败重试 1 次（§3.2.4）
		retry, err2 := provider.Chat(c.Request.Context(), llm.ChatOpts{
			Messages: llm.BuildPlanRetryPrompt(ctx, result.Content, reason), JSONMode: true,
			Temperature: 0.2, Thinking: true, MaxTokens: 4000,
		})
		if err2 != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "llm_error", "message": "生成失败，请重试"})
			return
		}
		doc, reason = validatePlan(retry.Content, ctx)
	}
	if doc == nil { // 仍失败 → 回 S6（§3.2.4）
		sess.Stage = string(fsm.S6)
		_ = db.SaveSessionStage(sess)
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "plan_invalid",
			"message": "这次没整理好，回到确认步骤换个说法再试", "detail": reason})
		return
	}

	raw, _ := json.Marshal(doc)
	planID, err := db.InsertPlan(user.ID, sess.ID, string(raw))
	if err != nil {
		c.JSON(500, gin.H{"error": "internal", "message": "计划入库失败"})
		return
	}
	sess.Stage, sess.Status = string(fsm.Active), "done"
	if ctx.Wish != "" {
		sess.Wish = &ctx.Wish
	}
	_ = db.SaveSessionStage(sess)
	db.UsageIncr(user.ID, db.Today(), "plan_generations", 0)

	c.JSON(http.StatusOK, gin.H{"plan": doc, "planId": planID})
}

// validatePlan 服务端强校验（§3.3.1 + S7.md 规则）
func validatePlan(content string, ctx *fsm.Context) (*planDoc, string) {
	clean := strings.TrimSpace(fenceRe.ReplaceAllString(strings.TrimSpace(content), ""))
	var doc planDoc
	if err := json.Unmarshal([]byte(clean), &doc); err != nil {
		return nil, "JSON 解析失败: " + err.Error()
	}
	if n := len([]rune(doc.Wish)); n == 0 || n > 30 {
		return nil, "wish 长度非法"
	}
	if len(doc.Habits) < 1 || len(doc.Habits) > 3 {
		return nil, "habits 数量须 1-3"
	}
	if len(doc.Habits) != len(ctx.Golden) {
		return nil, "habits 数量与选定黄金行为数不一致"
	}
	for i, h := range doc.Habits {
		if len([]rune(h.Title)) > 12 {
			return nil, "习惯标题超12字"
		}
		if !strings.HasPrefix(h.Anchor, "在我") {
			return nil, "锚点须「在我X之后」句式"
		}
		if n := len([]rune(h.Behavior)); n == 0 || n > 20 {
			return nil, "微行为须1-20字"
		}
		if h.Celebration == "" {
			return nil, "缺少庆祝方式"
		}
		if !regexp.MustCompile(`^\d{2}:\d{2}$`).MatchString(h.AnchorTime) {
			return nil, "anchor_time 须 HH:mm"
		}
		if len(h.Progression) != 1 {
			return nil, "progression 须恰好1级"
		}
		if ac, ok := h.Progression[0].AfterCheckins.(float64); !ok || int(ac) != 7 {
			return nil, "progression.after_checkins 须为 7"
		}
		_ = i
	}
	days := map[string]bool{"monday": true, "tuesday": true, "wednesday": true, "thursday": true,
		"friday": true, "saturday": true, "sunday": true}
	if !days[doc.ReviewDay] {
		return nil, "review_day 非法"
	}
	if len([]rune(doc.CoachNote)) > 60 {
		return nil, "coach_note 超60字"
	}
	return &doc, ""
}
