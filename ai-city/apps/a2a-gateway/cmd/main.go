// Package main A2A Gateway gRPC 入口（Sprint 5 MVP + Sprint 5.5 信任 + 路由）。
//
// 启动：监听 A2A_GRPC_ADDR（默认 127.0.0.1:50061），注册 A2AGatewayServer。
// 设计：docs/06-A2A协议.md §20；06-A2A-canonical.md（签名规范）。
//
// 环境变量：
//   A2A_GRPC_ADDR          默认 127.0.0.1:50061
//   A2A_REPLAY_WINDOW_SEC  ed25519 重放窗口秒数，默认 300
package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/aicity/a2a-gateway/internal/a2asrv"
	a2av1 "github.com/aicity/proto/gen/go/a2a/v1"
	"google.golang.org/grpc"
)

func main() {
	addr := os.Getenv("A2A_GRPC_ADDR")
	if addr == "" {
		addr = "127.0.0.1:50061"
	}
	replaySec := parseReplayWindow()
	log.Printf("a2a-gateway starting gRPC on %s (replay_window=%ds)", addr, int(replaySec.Seconds()))

	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("listen %s: %v", addr, err)
	}

	srv := grpc.NewServer()
	reg := a2asrv.NewRegistry()
	verifier := a2asrv.NewVerifier(replaySec)
	dispatcher := a2asrv.NewDispatcher()
	dispatcher.Register(a2asrv.EchoAdapter{})
	dispatcher.Register(a2asrv.OpenClawStub{})
	dispatcher.Register(a2asrv.WorkbuddyStub{})
	dispatcher.SetFallback(a2asrv.EchoAdapter{})

	a2av1.RegisterA2AGatewayServer(srv, a2asrv.NewService(reg, verifier, dispatcher))

	// Graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		if err := srv.Serve(lis); err != nil {
			log.Printf("grpc.Serve: %v", err)
		}
	}()
	log.Printf("a2a-gateway ready (registry size=%d)", reg.Size())

	<-ctx.Done()
	log.Printf("a2a-gateway shutting down ...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stopped := make(chan struct{})
	go func() {
		srv.GracefulStop()
		close(stopped)
	}()
	select {
	case <-stopped:
		log.Printf("a2a-gateway stopped cleanly")
	case <-shutdownCtx.Done():
		log.Printf("a2a-gateway force stop (timeout)")
		srv.Stop()
	}
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
