// Package router 注册所有路由
package router

import (
	"log"

	"github.com/aicity/api-gateway/internal/config"
	"github.com/aicity/api-gateway/internal/handlers"
	"github.com/aicity/api-gateway/internal/middleware"
	"github.com/aicity/api-gateway/internal/store"
	"github.com/aicity/api-gateway/internal/worldgrpc"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func Register(r *gin.Engine, cfg *config.Config, db *pgxpool.Pool, playerStore *store.PlayerStore, worldClient *worldgrpc.Client, npcTalkHandler *handlers.NPCTalkHandler, npcPositionHandler *handlers.NPCPositionHandler) {
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "service": cfg.ServiceName})
	})

	// Sprint 2: Prometheus metrics 端点
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// 初始化 handlers（playerStore 由 main 注入，避免重复创建）
	authHandler := handlers.NewAuthHandler(playerStore, db, cfg.JWTSecret, cfg.JWTExpiry)
	playerHandler := handlers.NewPlayerHandler(playerStore)
	worldMoveHandler := handlers.NewWorldMoveHandler(worldClient)
	worldProxy, err := handlers.NewWorldProxy(cfg.WorldURL)
	if err != nil {
		log.Fatalf("invalid WORLD_ENGINE_URL %q: %v", cfg.WorldURL, err)
	}

	// 公开路由（无需鉴权）
	public := r.Group("/v1")
	{
		public.POST("/auth/login", authHandler.Login)
		public.POST("/auth/register", authHandler.Register)
		// /city is a guest-preview surface: movement uses a browser-local
		// guest id, while account features still require the authed routes.
		public.POST("/world/move", worldMoveHandler.Move)
		// The city preview loads a read-only world snapshot without a JWT.
		// Mutations still go through the public move endpoint or authed APIs.
		public.GET("/tiles", worldProxy.Proxy)
		// Scripted NPC talk is part of the guest city preview. The caller
		// explicitly supplies player_id and can only walk a read-only tree.
		public.POST("/npc/:id/talk", npcTalkHandler.HandleByID)
	}

	// 鉴权路由
	authed := r.Group("/v1")
	authed.Use(middleware.Auth(cfg.JWTSecret))
	{
		// 玩家相关
		authed.GET("/players/me", playerHandler.Me)
		authed.GET("/players/:id", playerHandler.GetByID)

		// NPC 相关
		// 1.0 必新 endpoint（acceptance_1_0 §一.B）：GET/POST /v1/npc/:id/position
		authed.GET("/npc/:id/position", npcPositionHandler.HandleGet)
		authed.POST("/npc/:id/position", npcPositionHandler.HandleSet)
		// NPC 初始节点（say + options）—— Web 点 NPC 时可拉
		authed.GET("/npcs/:id", npcTalkHandler.HandleInfo)

		// 剧本相关（占位）
		authed.POST("/sagas", func(c *gin.Context) { c.JSON(501, gin.H{"error": "TODO"}) })
		authed.GET("/sagas/:id", func(c *gin.Context) { c.JSON(501, gin.H{"error": "TODO"}) })

		// 世界相关：读路径（tiles）继续走 REST proxy（web 用，便于缓存），
		// 写路径（move）走 gRPC（Sprint 3.5）
		authed.GET("/tiles/:id", worldProxy.Proxy)
	}
}
