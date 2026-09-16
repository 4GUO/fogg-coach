package main

import (
	"log"

	"fogg-coach/config"
	"fogg-coach/db"
	"fogg-coach/middleware"
	"fogg-coach/routes"

	"github.com/gin-gonic/gin"
)

func main() {
	cfg := config.Load()
	if !cfg.Debug {
		gin.SetMode(gin.ReleaseMode)
	}
	if err := db.Open("fogg-coach.db"); err != nil {
		log.Fatalf("[fatal] %v", err)
	}

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	api := r.Group("/api")
	api.GET("/health", routes.Health)

	// 登录（无需鉴权）
	api.POST("/auth/login", routes.Login)

	// 鉴权区
	authed := api.Group("", middleware.Auth())
	authed.GET("/me", routes.Me)

	log.Printf("[fogg-coach] 监听 :%s（WX_MOCK=%v）", cfg.Port, cfg.WXMock)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatalf("[fatal] %v", err)
	}
}
