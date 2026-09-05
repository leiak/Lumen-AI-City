// Package main A2A Gateway gRPC 入口（Sprint 5 MVP）。
//
// 启动：监听 A2A_GRPC_ADDR（默认 127.0.0.1:50061），注册 A2AGatewayServer。
// 设计：docs/06-A2A协议.md §20；MVP 范围见 SPRINT-5.md。
package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
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
	log.Printf("a2a-gateway starting gRPC on %s", addr)

	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("listen %s: %v", addr, err)
	}

	srv := grpc.NewServer()
	reg := a2asrv.NewRegistry()
	a2av1.RegisterA2AGatewayServer(srv, a2asrv.NewService(reg))

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
