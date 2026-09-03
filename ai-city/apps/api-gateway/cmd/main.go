// Package main 启动 API Gateway
// 职责：路由 / 限流 / 鉴权 / 反爬 / 配额 / Trace 注入
// 详细设计见 docs/04-API设计.md §18.7
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aicity/api-gateway/internal/config"
	"github.com/aicity/api-gateway/internal/middleware"
	"github.com/aicity/api-gateway/internal/router"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func main() {
	cfg := config.Load()
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	// 中间件链（顺序关键）
	r.Use(middleware.Recovery(logger))
	r.Use(middleware.TraceID())
	r.Use(middleware.Logging(logger))
	r.Use(middleware.RateLimit(cfg.RedisURL))
	r.Use(middleware.Auth(cfg.JWTSecret))
	r.Use(middleware.AntiScrap())

	router.Register(r, cfg)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("api-gateway starting", zap.String("port", cfg.Port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("listen failed", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("shutdown failed", zap.Error(err))
	}
}
