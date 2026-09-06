// Package main A2A Gateway 双协议入口（Sprint 5 + 5.5 + 6 + 7 + 7+）。
//
// 启动：监听 gRPC (A2A_GRPC_ADDR) + HTTP (A2A_HTTP_ADDR) 两个端口，
// 共用 *a2asrv.Service 单例；后台启动 a2a_inbox TTL cleanup cron。
//
// Sprint 7：注入 PG-backed CardStore + InboxStore；EchoAdapter 替换为 InboxAdapter。
// Sprint 7+：InboxStore 加 defaultTTL；启动 cron 周期 DELETE expires_at < NOW()。
//
// 设计：docs/06-A2A协议.md §20；06-A2A-canonical.md（签名规范）。
//
// 环境变量：
//   A2A_GRPC_ADDR                    默认 127.0.0.1:50061
//   A2A_HTTP_ADDR                    默认 127.0.0.1:8083（HTTP gateway）
//   A2A_HTTP_API_KEY                 非空 = 启用 Bearer 鉴权（dev 留空）
//   A2A_REPLAY_WINDOW_SEC            ed25519 重放窗口秒数，默认 300
//   A2A_INBOX_TTL_HOURS              inbox 行 TTL 小时数，默认 168（7 天）；0 = 拒绝接受
//   A2A_INBOX_CLEANUP_INTERVAL_SEC   cleanup cron 间隔秒数，默认 300；0 = 禁用
//   DATABASE_URL                     PG 连接串（默认 postgresql://aicity:aicity_dev@localhost:5432/aicity）
package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/aicity/a2a-gateway/internal/a2asrv"
	"github.com/aicity/a2a-gateway/internal/httpgw"
	"github.com/jackc/pgx/v5/pgxpool"
	a2av1 "github.com/aicity/proto/gen/go/a2a/v1"
	"google.golang.org/grpc"
)

func main() {
	grpcAddr := getEnv("A2A_GRPC_ADDR", "127.0.0.1:50061")
	httpAddr := getEnv("A2A_HTTP_ADDR", "127.0.0.1:8083")
	apiKey := os.Getenv("A2A_HTTP_API_KEY")
	replaySec := parseReplayWindow()
	inboxTTL := parseInboxTTL()
	cleanupInterval := parseCleanupInterval()
	dbURL := getEnv("DATABASE_URL", "postgresql://aicity:aicity_dev@localhost:5432/aicity")

	log.Printf("a2a-gateway starting: grpc=%s http=%s (replay_window=%ds api_key=%s db=%s inbox_ttl=%s cleanup_interval=%s)",
		grpcAddr, httpAddr, int(replaySec.Seconds()), redactKey(apiKey), redactDSN(dbURL), inboxTTL, cleanupInterval)

	// PG 连接（启动期 10s ctx；仿 api-gateway cmd/main.go:34）
	bootCtx, bootCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer bootCancel()
	pool, err := pgxpool.New(bootCtx, dbURL)
	if err != nil {
		log.Fatalf("pg connect: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(bootCtx); err != nil {
		log.Fatalf("pg ping: %v", err)
	}
	cardStore := a2asrv.NewCardStore(pool)
	inboxStore := a2asrv.NewInboxStoreWithTTL(pool, inboxTTL)
	log.Printf("a2a-gateway: PG ready (card_store + inbox_store wired)")

	// 共享 Service
	reg := a2asrv.NewRegistryFromCardStore(cardStore)
	verifier := a2asrv.NewVerifier(replaySec)
	dispatcher := a2asrv.NewDispatcher()

	// InboxAdapter：aicity provider 的兜底；也作为 Dispatcher fallback
	inboxAdapter := a2asrv.NewInboxAdapter(inboxStore)

	// Outbound HTTP adapter：openclaw / workbuddy（共享 5s timeout client + inbox fallback）
	httpClient := a2asrv.NewHTTPClient(5 * time.Second)
	dispatcher.Register(a2asrv.NewHTTPAdapter("openclaw", "openclaw", httpClient, inboxStore))
	dispatcher.Register(a2asrv.NewHTTPAdapter("workbuddy", "workbuddy", httpClient, inboxStore))
	dispatcher.Register(inboxAdapter)
	dispatcher.SetFallback(inboxAdapter)

	svc := a2asrv.NewService(reg, verifier, dispatcher, inboxStore)

	// gRPC server
	grpcLis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		log.Fatalf("grpc listen %s: %v", grpcAddr, err)
	}
	grpcSrv := grpc.NewServer()
	a2av1.RegisterA2AGatewayServer(grpcSrv, svc)

	// HTTP server
	httpHandler := httpgw.New(svc, apiKey)
	httpSrv := &http.Server{
		Addr:              httpAddr,
		Handler:           httpHandler.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Sprint 7+：启动 inbox TTL cleanup 后台 cron（独立 long-lived ctx 模式；
	// 与 api-gateway subscriber 同坑 —— 不能复用启动期 bootCtx 否则 10s 后 ctx 自动取消）
	a2asrv.StartInboxCleanup(ctx, inboxStore, cleanupInterval)

	var wg sync.WaitGroup

	// gRPC serve goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Printf("a2a-gateway gRPC ready on %s", grpcAddr)
		if err := grpcSrv.Serve(grpcLis); err != nil {
			log.Printf("grpc.Serve: %v", err)
		}
	}()

	// HTTP serve goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Printf("a2a-gateway HTTP ready on %s (api_key=%s)", httpAddr, redactKey(apiKey))
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("http.Serve: %v", err)
		}
	}()

	log.Printf("a2a-gateway ready (registry size=%d)", reg.Size())

	<-ctx.Done()
	log.Printf("a2a-gateway shutting down ...")

	// 双协议并行 graceful shutdown（HTTP 5s 限时）
	httpShutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	httpStopped := make(chan struct{})
	go func() {
		if err := httpSrv.Shutdown(httpShutdownCtx); err != nil {
			log.Printf("http.Shutdown: %v", err)
		}
		close(httpStopped)
	}()

	grpcStopped := make(chan struct{})
	go func() {
		grpcSrv.GracefulStop()
		close(grpcStopped)
	}()

	// 等两个 server 都停（或超时）
	select {
	case <-grpcStopped:
		log.Printf("gRPC stopped")
	case <-httpShutdownCtx.Done():
		log.Printf("HTTP shutdown timeout, force stop")
		grpcSrv.Stop()
	}
	<-httpStopped
	log.Printf("HTTP stopped")

	// 等 serve goroutine 退出（理论上 Serve 已返）
	wg.Wait()
	log.Printf("a2a-gateway stopped cleanly")
}

// getEnv 读 env，空返 fallback。
func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// parseReplayWindow 读 A2A_REPLAY_WINDOW_SEC；非法/缺失 → 5min。
func parseReplayWindow() time.Duration {
	s := os.Getenv("A2A_REPLAY_WINDOW_SEC")
	if s == "" {
		return 5 * time.Minute
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		log.Printf("a2a-gateway: invalid A2A_REPLAY_WINDOW_SEC=%q, fallback to 300s", s)
		return 5 * time.Minute
	}
	return time.Duration(n) * time.Second
}

// parseInboxTTL 读 A2A_INBOX_TTL_HOURS；非法/缺失 → 168h（7 天）。
//
//   - n == 0 → 返 0 表示"PG DEFAULT 兜底"（不显式写 expires_at）
//   - n  < 0 → log + fallback
//   - 非法字符串 → log + fallback
func parseInboxTTL() time.Duration {
	s := os.Getenv("A2A_INBOX_TTL_HOURS")
	if s == "" {
		return 168 * time.Hour
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		log.Printf("a2a-gateway: invalid A2A_INBOX_TTL_HOURS=%q, fallback to 168h", s)
		return 168 * time.Hour
	}
	return time.Duration(n) * time.Hour
}

// parseCleanupInterval 读 A2A_INBOX_CLEANUP_INTERVAL_SEC；非法/缺失 → 300s。
//
//   - n == 0 → 返 0 表示"禁用 cron"（StartInboxCleanup 会 log）
//   - n  < 0 → log + fallback
func parseCleanupInterval() time.Duration {
	s := os.Getenv("A2A_INBOX_CLEANUP_INTERVAL_SEC")
	if s == "" {
		return 300 * time.Second
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		log.Printf("a2a-gateway: invalid A2A_INBOX_CLEANUP_INTERVAL_SEC=%q, fallback to 300s", s)
		return 300 * time.Second
	}
	return time.Duration(n) * time.Second
}

// redactKey 把 api key 缩成 "set" / "unset" 形式打印（避免日志泄露密钥）。
func redactKey(k string) string {
	if k == "" {
		return "unset"
	}
	return "set"
}

// redactDSN 把 PG 连接串的密码部分打码（保留 user@host:port/db）。
func redactDSN(dsn string) string {
	// 简化：仅保留 scheme://user:***@host:port/db 形式
	// 完整 DSN 解析交给 pgx；这里只脱敏
	if i := indexOf(dsn, "://"); i >= 0 {
		rest := dsn[i+3:]
		if at := indexOf(rest, "@"); at >= 0 {
			userHost := rest[:at]
			hostPart := rest[at+1:]
			// user:pass → user:***
			if colon := indexOf(userHost, ":"); colon >= 0 {
				return dsn[:i+3] + userHost[:colon+1] + "***@" + hostPart
			}
		}
	}
	return dsn
}

// indexOf 简化版 strings.Index（避免引入 strings import 噪声）。
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
