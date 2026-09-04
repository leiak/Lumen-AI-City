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
	"github.com/aicity/api-gateway/internal/store"
	"github.com/aicity/api-gateway/internal/subscriber"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

func main() {
	cfg := config.Load()
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	// 连接 PG（独立短超时 ctx，只用于启动期）
	bootCtx, bootCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer bootCancel()
	db, err := pgxpool.New(bootCtx, cfg.DatabaseURL)
	if err != nil {
		logger.Fatal("pg connect failed", zap.Error(err))
	}
	defer db.Close()
	if err := db.Ping(bootCtx); err != nil {
		logger.Fatal("pg ping failed", zap.Error(err))
	}
	logger.Info("pg connected")

	// Sprint 2: 构造 Prometheus 默认 collectors（构造时自动注册到 default registry）
	// 注意：NewGoCollector / NewProcessCollector 已经自带 MustRegister，
	//       这里只是显式持有引用以便未来反注册。
	_ = collectors.NewGoCollector()
	_ = collectors.NewProcessCollector(collectors.ProcessCollectorOpts{})

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	// 中间件链（顺序关键）
	r.Use(middleware.Recovery(logger))
	r.Use(middleware.TraceID())
	r.Use(middleware.Logging(logger))
	r.Use(middleware.RateLimit(cfg.RedisURL))
	r.Use(middleware.AntiScrap())

	// Redis Pub/Sub 订阅者：消费 aicity:player:moved → 写 PG player_position
	redisOpt, redisErr := redis.ParseURL(cfg.RedisURL)
	if redisErr != nil {
		logger.Fatal("redis url parse failed", zap.Error(redisErr))
	}
	rdb := redis.NewClient(redisOpt)
	if err := rdb.Ping(bootCtx).Err(); err != nil {
		logger.Fatal("redis ping failed", zap.Error(err))
	}
	logger.Info("redis connected", zap.String("url", cfg.RedisURL))

	// 服务期 ctx：跟随 SIGINT/SIGTERM 取消，subscriber 才会干净退出
	appCtx, appCancel := context.WithCancel(context.Background())
	defer appCancel()

	playerStore := store.NewPlayerStore(db)
	subscriber.PlayerMoved(appCtx, rdb, "aicity:player:moved", playerStore, logger)

	router.Register(r, cfg, db, playerStore)

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

	// 先取消 appCtx 让 subscriber 退出，再 graceful shutdown HTTP
	appCancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatal("shutdown failed", zap.Error(err))
	}
}