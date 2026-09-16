package routes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"fogg-coach/config"
	"fogg-coach/db"
	"fogg-coach/fsm"
	"fogg-coach/llm"
	"fogg-coach/middleware"

	"github.com/gin-gonic/gin"
)

// POST /api/chat — SSE 流式对话（§4.1）+ FSM 集成
type chatAction struct {
	Type    string   `json:"type"` // select | confirm | revise | reset_wish
	Options []string `json:"options"`
}

type chatReq struct {
	SessionID string      `json:"sessionId"`
	Content   string      `json:"content"`
	Action    *chatAction `json:"action"`
}

var provider llm.Provider // main 里注入

func SetLLM(p llm.Provider) { provider = p }

func Chat(c *gin.Context) {
	user := middleware.CurrentUser(c)
	cfg := config.Get()

	var req chatReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "请求体非法"})
		return
	}

	// 会话定位 / 创建（首 session 未结束不可开新的）
	var sess *db.Session
	if req.SessionID != "" {
		s, err := db.SessionByID(req.SessionID)
		if err == db.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found", "message": "会话不存在"})
			return
		}
		sess = s
	} else {
		if existing, err := db.ActiveSessionByUser(user.ID); err == nil {
			c.JSON(http.StatusConflict, gin.H{"error": "session_exists",
				"message": "当前咨询还未结束，请继续完成", "sessionId": existing.ID})
			return
		}
		s, err := db.CreateSession(user.ID)
		if err != nil {
			c.JSON(500, gin.H{"error": "internal", "message": "创建会话失败"})
			return
		}
		sess = s
	}

	// SSE 通道
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("X-Accel-Buffering", "no") // Nginx 不缓冲
	flush := func() {
		if f, ok := c.Writer.(http.Flusher); ok {
			f.Flush()
		}
	}
	send := func(v any) {
		b, _ := json.Marshal(v)
		fmt.Fprintf(c.Writer, "data: %s\n\n", b)
		flush()
	}

	stage := fsm.Stage(sess.Stage)
	ctx := fsm.ParseContext(sess.Context)
	turns := db.CountUserTurns(sess.ID)

	// ---- 按钮事件（绕过 LLM，§3.1.3 防线3） ----
	if req.Action != nil {
		var d fsm.Decision
		switch req.Action.Type {
		case "select":
			d = fsm.OnSelect(stage, ctx, req.Action.Options)
		case "confirm": // S6 确认 → S7，前端随后调 /plan/generate
			d = fsm.OnConfirm(stage)
		case "revise":
			d = fsm.OnRevise(stage, ctx)
		case "reset_wish":
			if sess.ResetCount >= 2 { // 每 session ≤2 次（§3.1.5）
				send(gin.H{"delta": "咱们先把当前这个愿望走完吧，之后随时能开新的。"})
				send(gin.H{"done": true, "sessionId": sess.ID, "stage": string(stage)})
				return
			}
			d = fsm.OnResetWish(stage, ctx)
			sess.ResetCount++
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "未知 action"})
			return
		}
		if d.Next != stage {
			sess.Stage, stage = string(d.Next), d.Next
		}
		if req.Action.Type == "confirm" && d.Action == "advance" {
			_ = db.SaveSessionStage(sess)
			send(gin.H{"event": "stage_done", "next": "S7"})
			send(gin.H{"done": true, "sessionId": sess.ID, "stage": "S7"})
			return
		}
		if req.Content == "" {
			sess.Context = ctx.JSON() // action 已修改 ctx（select/reset 等），落库防丢
			_ = db.SaveSessionStage(sess)
			if d.Next != fsm.S5 && d.Next != fsm.S2 { // select/revise 后由前端发文本继续
				send(gin.H{"event": "stage_done", "next": string(d.Next)})
			}
			send(gin.H{"done": true, "sessionId": sess.ID, "stage": string(stage)})
			return
		}
		// 带文本的动作（如 chips），落库后继续 LLM 轮
	}

	// ---- 轮次配额：25 轮兜底强制出计划（§3.5） ----
	if turns >= cfg.Quota.SessionTurns {
		fsm.ForceFill(ctx)
		sess.Stage, stage, sess.Context = "S6", fsm.S6, ctx.JSON()
		_ = db.SaveSessionStage(sess)
		send(gin.H{"delta": "咱们今天聊得足够多啦，我直接帮你把计划整理好，稍等。"})
		send(gin.H{"event": "stage_done", "next": "S7"})
		send(gin.H{"done": true, "forcePlan": true, "sessionId": sess.ID})
		return
	}

	// ---- 配额：日 LLM 消息 ----
	if !middleware.CheckDailyQuota(c) {
		return
	}

	// ---- LLM 对话轮 ----
	userInput := strings.TrimSpace(req.Content)
	if userInput == "" {
		send(gin.H{"error": "empty_input", "message": "说点什么吧"})
		send(gin.H{"done": true, "sessionId": sess.ID, "stage": string(stage)})
		return
	}
	if n := len([]rune(userInput)); n > cfg.Quota.MessageMaxLen { // 500 字截断
		userInput = string([]rune(userInput)[:cfg.Quota.MessageMaxLen])
	}

	history := db.ListMessages(sess.ID)
	_, _ = db.InsertMessage(sess.ID, "user", userInput, "")

	// 执行期阶段（active/S8/S9）M4 再实现
	if stage == fsm.Active || stage == fsm.S7 || stage == fsm.S8 || stage == fsm.S9 {
		send(gin.H{"error": "stage_invalid", "message": "当前阶段不支持对话，请直接生成计划"})
		send(gin.H{"done": true, "sessionId": sess.ID, "stage": string(stage)})
		return
	}

	msgs := llm.BuildChatPrompt(stage, ctx, toHistory(history), userInput)
	result, err := provider.Chat(c.Request.Context(), llm.ChatOpts{
		Messages: msgs, Temperature: 0.7, MaxTokens: 2500,
		OnDelta: func(s string) { send(gin.H{"delta": s}) }, // §3.2.5：S1-S6 thinking 已在 Provider 内默认关闭
	})
	if err != nil {
		send(gin.H{"error": "llm_error", "message": "教练开小差了，请重试一次"})
		send(gin.H{"done": true, "sessionId": sess.ID, "stage": string(stage)})
		return
	}

	// 信息抽取（轻量 JSON 调用，失败不致命）→ FSM 裁决
	visible := fsm.StripMarkers(result.Content)
	if upd, e := provider.Chat(c.Request.Context(), llm.ChatOpts{
		Messages: llm.BuildExtractPrompt(stage, ctx, append(toHistory(history), llm.HistoryMsg{Role: "assistant", Content: visible})),
		JSONMode: true, Temperature: 0, MaxTokens: 500,
	}); e == nil {
		llm.MergeExtract(ctx, upd.Content)
	}

	d := fsm.Evaluate(stage, ctx, result.Content)
	// 后端复评：context 硬条件已齐但 LLM 未标 DONE → 后端直接推进（推进权在后端）
	if d.Action == "hold" && stage != fsm.S4 && stage != fsm.S6 && fsm.CanAdvance(stage, ctx) {
		if nxt := fsm.CanAdvanceTo(stage); nxt != "" {
			d = fsm.Decision{Next: nxt, Action: "advance", QuickReplies: d.QuickReplies}
		}
	}
	meta := "{}"
	if len(d.QuickReplies) > 0 {
		mb, _ := json.Marshal(map[string]any{"quick_replies": d.QuickReplies})
		meta = string(mb)
	}
	msgID, _ := db.InsertMessage(sess.ID, "assistant", visible, meta)

	if d.Next != stage {
		sess.Stage = string(d.Next)
	}
	sess.Context = ctx.JSON()
	if d.Action == "reset_wish" {
		sess.ResetCount++
	}
	if stage == fsm.S1 && d.Next == fsm.S2 && ctx.Wish != "" {
		sess.Wish = &ctx.Wish
	}
	_ = db.SaveSessionStage(sess)
	db.UsageIncr(user.ID, db.Today(), "llm_messages", 0)

	if len(d.QuickReplies) > 0 {
		send(gin.H{"event": "quick_replies", "items": d.QuickReplies})
	}
	if d.Next != stage {
		send(gin.H{"event": "stage_done", "next": string(d.Next)})
	}
	send(gin.H{"done": true, "messageId": msgID, "sessionId": sess.ID, "stage": string(d.Next)})
	_ = time.Now()
}

func toHistory(rows []struct{ Role, Content string }) []llm.HistoryMsg {
	out := make([]llm.HistoryMsg, 0, len(rows))
	for _, r := range rows {
		out = append(out, llm.HistoryMsg{Role: r.Role, Content: r.Content})
	}
	return out
}
