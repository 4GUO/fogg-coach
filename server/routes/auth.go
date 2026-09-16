package routes

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"fogg-coach/config"
	"fogg-coach/db"
	"fogg-coach/middleware"

	"github.com/gin-gonic/gin"
)

// POST /api/auth/login — 双 provider 统一入口（§3.6/§4.0）
// M1 实现 wechat_mp（+WX_MOCK）；phone 端 M3+（短信服务接入）再补
type loginReq struct {
	Provider string `json:"provider" binding:"required"` // wechat_mp | phone
	Code     string `json:"code"`                        // wechat_mp: wx.login code
	Phone    string `json:"phone"`                       // phone 端（未实现）
	SMSCode  string `json:"smsCode"`
}

type wxSessionResp struct {
	OpenID     string `json:"openid"`
	SessionKey string `json:"session_key"`
	ErrCode    int    `json:"errcode"`
}

func Login(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "缺少 provider"})
		return
	}
	switch req.Provider {
	case "wechat_mp":
		loginWechatMP(c, req.Code)
	case "phone":
		c.JSON(http.StatusNotImplemented, gin.H{"error": "not_implemented", "message": "手机号登录 M3+ 开放，敬请期待"})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "未知 provider: " + req.Provider})
	}
}

func loginWechatMP(c *gin.Context, code string) {
	cfg := config.Get()
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "缺少 code"})
		return
	}
	var openid, sessionKey string
	if cfg.WXMock { // 开发模式：任意 code 可登（沿用 legacy 模式）
		openid, sessionKey = "mock_"+code, "mock_session_key"
	} else {
		resp, err := http.Get(fmt.Sprintf(
			"https://api.weixin.qq.com/sns/jscode2session?appid=%s&secret=%s&js_code=%s&grant_type=authorization_code",
			cfg.WXAppID, cfg.WXSecret, urlQueryEscape(code)))
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "wx_error", "message": "微信接口不可达"})
			return
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		var wx wxSessionResp
		if json.Unmarshal(body, &wx) != nil || wx.OpenID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "wx_error", "message": "code 换取失败", "detail": wx.ErrCode})
			return
		}
		openid, sessionKey = wx.OpenID, wx.SessionKey
	}

	user, err := db.UpsertByIdentity("wechat_mp", openid, sessionKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal", "message": "登录失败"})
		return
	}
	token, exp, err := middleware.SignToken(user.ID, user.TokenVer)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal", "message": "签发 token 失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"token":     token,
		"expiresAt": exp.Format(time.RFC3339),
		"user":      gin.H{"userId": user.ID, "nickname": user.Nickname},
	})
}

func urlQueryEscape(s string) string {
	// 微信 code 仅含字母数字下划线横线，简单转义足够；避免引入 net/url 命名冲突
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_' || ch == '.' || ch == '~' {
			out = append(out, ch)
		} else {
			out = append(out, fmt.Sprintf("%%%02X", ch)...)
		}
	}
	return string(out)
}
