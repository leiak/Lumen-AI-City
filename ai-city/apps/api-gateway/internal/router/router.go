// Package router 注册所有路由
package router

import (
	"log"

	"github.com/aicity/api-gateway/internal/config"
	"github.com/aicity/api-gateway/internal/handlers"
	"github.com/aicity/api-gateway/internal/middleware"
	"github.com/aicity/api-gateway/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func Register(r *gin.Engine, cfg *config.Config, db *pgxpool.Pool, playerStore *store.PlayerStore) {
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "service": cfg.ServiceName})
	})

	// 初始化 handlers（playerStore 由 main 注入，避免重复创建）
	authHandler := handlers.NewAuthHandler(playerStore, db, cfg.JWTSecret, cfg.JWTExpiry)
	playerHandler := handlers.NewPlayerHandler(playerStore)
	worldProxy, err := handlers.NewWorldProxy(cfg.WorldURL)
	if err != nil {
		log.Fatalf("invalid WORLD_ENGINE_URL %q: %v", cfg.WorldURL, err)
	}

	// 公开路由（无需鉴权）
	public := r.Group("/v1")
	{
		public.POST("/auth/login", authHandler.Login)
		public.POST("/auth/register", authHandler.Register)
	}

	// 鉴权路由
	authed := r.Group("/v1")
	authed.Use(middleware.Auth(cfg.JWTSecret))
	{
		// 玩家相关
		authed.GET("/players/me", playerHandler.Me)
		authed.GET("/players/:id", playerHandler.GetByID)

		// NPC 相关（占位）
		authed.GET("/npcs/:id", func(c *gin.Context) { c.JSON(501, gin.H{"error": "TODO"}) })
		authed.POST("/npcs/:id/dialogue", func(c *gin.Context) { c.JSON(501, gin.H{"error": "TODO"}) })

		// 剧本相关（占位）
		authed.POST("/sagas", func(c *gin.Context) { c.JSON(501, gin.H{"error": "TODO"}) })
		authed.GET("/sagas/:id", func(c *gin.Context) { c.JSON(501, gin.H{"error": "TODO"}) })

		// 世界相关：反向代理到 world-engine（Sprint 1 接入）
		authed.GET("/tiles", worldProxy.Proxy)
		authed.GET("/tiles/:id", worldProxy.Proxy)
		authed.POST("/world/move", worldProxy.Proxy)
	}
}