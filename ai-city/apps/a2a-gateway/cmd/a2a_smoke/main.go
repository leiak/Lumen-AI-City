// Package main A2A Gateway E2E smoke（仿 api-gateway cmd/grpc_smoke 模式）。
//
// 用法：
//   1) 启动 server： A2A_GRPC_ADDR=127.0.0.1:50061 go run ./cmd/main.go
//   2) 跑 smoke：   go run ./cmd/a2a_smoke
//
// 退出码：0 = 全部 OK；非 0 = 失败（CI / 手动验收用）。
//
// 检查清单：
//   [OK] RegisterCard alice / bob
//   [OK] Discover capability=chat → 2 cards
//   [OK] SendMessage alice → bob delivered=true
//   [OK] SendMessage alice → ghost delivered=false error=F_004
//   [OK] Stream echo 3 messages
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	a2av1 "github.com/aicity/proto/gen/go/a2a/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	addr := os.Getenv("A2A_GRPC_ADDR")
	if addr == "" {
		addr = "127.0.0.1:50061"
	}

	fmt.Printf("[a2a_smoke] dial %s ...\n", addr)
	conn, err := grpc.NewClient(
		addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		fmt.Printf("[FAIL] dial: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()
	c := a2av1.NewA2AGatewayClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1) RegisterCard alice + bob
	if _, err := c.RegisterCard(ctx, &a2av1.AgentCard{
		AgentId: "alice", Name: "Alice", Provider: "aicity", Version: "1.0",
		Capabilities: []string{"chat", "search"},
	}); err != nil {
		fmt.Printf("[FAIL] register alice: %v\n", err)
		os.Exit(2)
	}
	if _, err := c.RegisterCard(ctx, &a2av1.AgentCard{
		AgentId: "bob", Name: "Bob", Provider: "aicity", Version: "1.0",
		Capabilities: []string{"chat"},
	}); err != nil {
		fmt.Printf("[FAIL] register bob: %v\n", err)
		os.Exit(3)
	}
	fmt.Println("[OK]   RegisterCard alice / bob")

	// 2) Discover chat → 2 cards
	disc, err := c.Discover(ctx, &a2av1.DiscoverRequest{Capability: "chat"})
	if err != nil {
		fmt.Printf("[FAIL] Discover chat: %v\n", err)
		os.Exit(4)
	}
	if got := len(disc.GetCards()); got != 2 {
		fmt.Printf("[FAIL] Discover chat want 2 got %d\n", got)
		os.Exit(5)
	}
	fmt.Printf("[OK]   Discover capability=chat → 2 cards\n")

	// 3) SendMessage alice → bob delivered=true
	resp, err := c.SendMessage(ctx, &a2av1.Message{
		MessageId: "m_ok", FromAgentId: "alice", ToAgentId: "bob",
		Type: "request", Payload: []byte("hi bob"),
	})
	if err != nil {
		fmt.Printf("[FAIL] SendMessage ok: %v\n", err)
		os.Exit(6)
	}
	if !resp.GetDelivered() || resp.GetError() != "" {
		fmt.Printf("[FAIL] SendMessage ok delivered=%v err=%q\n", resp.GetDelivered(), resp.GetError())
		os.Exit(7)
	}
	fmt.Printf("[OK]   SendMessage alice → bob delivered=true\n")

	// 4) SendMessage alice → ghost delivered=false error=F_004
	resp, err = c.SendMessage(ctx, &a2av1.Message{
		MessageId: "m_bad", FromAgentId: "alice", ToAgentId: "ghost",
	})
	if err != nil {
		fmt.Printf("[FAIL] SendMessage ghost: %v\n", err)
		os.Exit(8)
	}
	if resp.GetDelivered() {
		fmt.Printf("[FAIL] ghost should not be delivered\n")
		os.Exit(9)
	}
	if got := resp.GetError(); len(got) < 5 || got[:5] != "F_004" {
		fmt.Printf("[FAIL] error want F_004 prefix, got %q\n", got)
		os.Exit(10)
	}
	fmt.Printf("[OK]   SendMessage alice → ghost delivered=false error=F_004\n")

	// 5) Stream echo 3 messages
	stream, err := c.Stream(ctx)
	if err != nil {
		fmt.Printf("[FAIL] Stream open: %v\n", err)
		os.Exit(11)
	}
	msgs := []*a2av1.Message{
		{MessageId: "s1", FromAgentId: "alice", ToAgentId: "bob", Type: "request", Payload: []byte("a")},
		{MessageId: "s2", FromAgentId: "alice", ToAgentId: "bob", Type: "request", Payload: []byte("b")},
		{MessageId: "s3", FromAgentId: "alice", ToAgentId: "bob", Type: "request", Payload: []byte("c")},
	}
	for _, m := range msgs {
		if err := stream.Send(m); err != nil {
			fmt.Printf("[FAIL] Stream send %s: %v\n", m.GetMessageId(), err)
			os.Exit(12)
		}
	}
	if err := stream.CloseSend(); err != nil {
		fmt.Printf("[FAIL] Stream CloseSend: %v\n", err)
		os.Exit(13)
	}
	for i, want := range msgs {
		got, err := stream.Recv()
		if err != nil {
			fmt.Printf("[FAIL] Stream Recv[%d]: %v\n", i, err)
			os.Exit(14)
		}
		if got.GetType() != "event" {
			fmt.Printf("[FAIL] Stream echo[%d] type want event got %q\n", i, got.GetType())
			os.Exit(15)
		}
		if string(got.GetPayload()) != string(want.GetPayload()) {
			fmt.Printf("[FAIL] Stream echo[%d] payload mismatch\n", i)
			os.Exit(16)
		}
	}
	// 流关闭：Recv 应返 EOF
	if _, err := stream.Recv(); err != nil && err != io.EOF {
		fmt.Printf("[FAIL] Stream EOF: %v\n", err)
		os.Exit(17)
	}
	fmt.Printf("[OK]   Stream echo 3 messages\n")

	fmt.Printf("\n[OK] all 5 a2a_smoke checks passed against %s\n", addr)
}
