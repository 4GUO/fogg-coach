package middleware

import (
	"net/http"
	"strings"
	"time"

	"fogg-coach/config"
	"fogg-coach/db"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// SignToken 签发 access JWT（payload: uid + ver，§3.6.2）
func SignToken(userID string, tokenVer int) (string, time.Time, error) {
	exp := time.Now().AddDate(0, 0, config.Get().JWTTTLDays)
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"uid": userID,
		"ver": tokenVer,
		"iat": time.Now().Unix(),
		"exp": exp.Unix(),
	})
	s, err := t.SignedString([]byte(config.Get().JWTSecret))
	return s, exp, err
}

// Auth Bearer 校验 + token_ver 吊销检查，挂载 user 到 context
func Auth() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			abort401(c, "缺少 token")
			return
		}
		tokenStr := strings.TrimPrefix(h, "Bearer ")
		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return []byte(config.Get().JWTSecret), nil
		})
		if err != nil || !token.Valid {
			abort401(c, "token 无效或过期")
			return
		}
		claims := token.Claims.(jwt.MapClaims)
		uid, _ := claims["uid"].(string)
		ver, _ := claims["ver"].(float64)
		if uid == "" {
			abort401(c, "token 无效")
			return
		}
		u, err := db.UserByID(uid)
		if err != nil { // 用户已删 → token 失效
			abort401(c, "token 无效")
			return
		}
		if int(ver) != u.TokenVer { // 吊销检查（§3.6.2）
			abort401(c, "token 已失效，请重新登录")
			return
		}
		c.Set("user", u)
		c.Next()
	}
}

func abort401(c *gin.Context, msg string) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized", "message": msg})
}

// CurrentUser 从 context 取已鉴权用户
func CurrentUser(c *gin.Context) *db.User {
	u, _ := c.Get("user")
	user, _ := u.(*db.User)
	return user
}
