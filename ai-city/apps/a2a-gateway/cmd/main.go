// Package main A2A Gateway 双协议入口（Sprint 5 + 5.5 + 6）。
//
// 启动：监听 gRPC (A2A_GRPC_ADDR) + HTTP (A2A_HTTP_ADDR) 两个端口，
// 共用 *a2asrv.Service 单例。
//
// 设计：docs/06-A2A协议.md §20；06-A2A-canonical.md（签名规范）。
//
// 环境变量：
//   A2A_GRPC_ADDR           默认 127.0.0.1:50061
//   A2A_HTTP_ADDR           默认 127.0.0.1:8083（HTTP gateway）
//   A2A_HTTP_API_KEY        非空 = 启用 Bearer 鉴权（dev 留空）
//   A2A_REPLAY_WINDOW_SEC   ed25519 重放窗口秒数，默认 300
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
	a2av1 "github.com/aicity/proto/gen/go/a2a/v1"
	"google.golang.org/grpc"
)

func main() {
	grpcAddr := getEnv("A2A_GRPC_ADDR", "127.0.0.1:50061")
	httpAddr := getEnv("A2A_HTTP_ADDR", "127.0.0.1:8083")
	apiKey := os.Getenv("A2A_HTTP_API_KEY")
	replaySec := parseReplayWindow()

	log.Printf("a2a-gateway starting: grpc=%s http=%s (replay_window=%ds api_key=%s)",
		grpcAddr, httpAddr, int(replaySec.Seconds()), redactKey(apiKey))

	// 共享 Service
	reg := a2asrv.NewRegistry()
	verifier := a2asrv.NewVerifier(replaySec)
	dispatcher := a2asrv.NewDispatcher()

	// Outbound HTTP adapter：openclaw / workbuddy（共享 5s timeout client）
	httpClient := a2asrv.NewHTTPClient(5 * time.Second)
	dispatcher.Register(a2asrv.NewHTTPAdapter("openclaw", "openclaw", httpClient))
	dispatcher.Register(a2asrv.NewHTTPAdapter("workbuddy", "workbuddy", httpClient))
	dispatcher.Register(a2asrv.EchoAdapter{})
	dispatcher.SetFallback(a2asrv.EchoAdapter{})

	svc := a2asrv.NewService(reg, verifier, dispatcher)

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

// redactKey 把 api key 缩成 "set" / "unset" 形式打印（避免日志泄露密钥）。
func redactKey(k string) string {
	if k == "" {
		return "unset"
	}
	return "set"
}
