package routes

import (
	"encoding/json"
	"net/http"

	"fogg-coach/db"
	"fogg-coach/fsm"
	"fogg-coach/middleware"

	"github.com/gin-gonic/gin"
)

// GET /api/session/active — 断线续聊：活跃会话 + 历史 + 前端所需 context 摘要
func ActiveSession(c *gin.Context) {
	user := middleware.CurrentUser(c)
	sess, err := db.ActiveSessionByUser(user.ID)
	if err == db.ErrNotFound {
		c.JSON(http.StatusNotFound, gin.H{"error": "no_session", "message": "没有进行中的会话"})
		return
	}
	ctx := fsm.ParseContext(sess.Context)
	msgs := db.ListMessages(sess.ID)
	type msg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
		Meta    string `json:"meta"`
	}
	// meta 单独查（ListMessages 不含），此处补齐
	rows, _ := db.DB.Query(`SELECT role, content, COALESCE(meta,'{}') FROM messages WHERE session_id = ? ORDER BY created_at, rowid`, sess.ID)
	list := []msg{}
	for rows.Next() {
		var m msg
		_ = rows.Scan(&m.Role, &m.Content, &m.Meta)
		list = append(list, m)
	}
	rows.Close()
	_ = msgs
	wish := ""
	if sess.Wish != nil {
		wish = *sess.Wish
	}
	c.JSON(http.StatusOK, gin.H{
		"session": gin.H{"id": sess.ID, "stage": sess.Stage, "wish": wish, "resetCount": sess.ResetCount},
		"messages": list,
		"context": gin.H{ // 前端渲染所需摘要（S4 候选等）
			"candidates": ctx.Candidates,
			"golden":     ctx.Golden,
		},
	})
}

// GET /api/plans — 用户计划列表（active 优先）
func ListPlans(c *gin.Context) {
	user := middleware.CurrentUser(c)
	rows, err := db.DB.Query(`SELECT id, status, version, data, created_at FROM plans
		WHERE user_id = ? ORDER BY status='active' DESC, created_at DESC`, user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
	defer rows.Close()
	type planRow struct {
		ID        string          `json:"id"`
		Status    string          `json:"status"`
		Version   int             `json:"version"`
		Data      json.RawMessage `json:"data"`
		CreatedAt string          `json:"createdAt"`
	}
	list := []planRow{}
	for rows.Next() {
		var p planRow
		if rows.Scan(&p.ID, &p.Status, &p.Version, &p.Data, &p.CreatedAt) == nil {
			list = append(list, p)
		}
	}
	c.JSON(http.StatusOK, gin.H{"plans": list})
}
