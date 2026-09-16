package routes

import (
	"net/http"
	"time"

	"fogg-coach/db"

	"github.com/gin-gonic/gin"
)

// GET /api/health
func Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"ok": true, "service": "fogg-coach", "time": time.Now().Format(time.RFC3339)})
}

// GET /api/me（鉴权后恢复用户态）
func Me(c *gin.Context) {
	u := mustUser(c)
	c.JSON(http.StatusOK, gin.H{
		"userId":   u.ID,
		"nickname": u.Nickname,
	})
}

func mustUser(c *gin.Context) *db.User {
	u, _ := c.Get("user")
	user, _ := u.(*db.User)
	return user
}
