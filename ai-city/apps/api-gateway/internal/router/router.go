// Package router 注册所有路由
package router

import (
	"github.com/aicity/api-gateway/internal/config"
	"github.com/gin-gonic/gin"
)

func Register(r *gin.Engine, cfg *config.Config) {
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "service": cfg.ServiceName})
	})

	// API v1 路由组
	v1 := r.Group("/v1")
	{
		// 玩家相关
		v1.POST("/auth/login", func(c *gin.Context) { c.JSON(501, gin.H{"error": "TODO"}) })
		v1.GET("/players/:id", func(c *gin.Context) { c.JSON(501, gin.H{"error": "TODO"}) })

		// NPC 相关
		v1.GET("/npcs/:id", func(c *gin.Context) { c.JSON(501, gin.H{"error": "TODO"}) })
		v1.POST("/npcs/:id/dialogue", func(c *gin.Context) { c.JSON(501, gin.H{"error": "TODO"}) })

		// 剧本相关
		v1.POST("/sagas", func(c *gin.Context) { c.JSON(501, gin.H{"error": "TODO"}) })
		v1.GET("/sagas/:id", func(c *gin.Context) { c.JSON(501, gin.H{"error": "TODO"}) })
	}
}
