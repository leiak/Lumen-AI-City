// Package main A2A Gateway E2E smoke（Sprint 5 5 项 + Sprint 5.5 7 项 = 12 项）。
//
// 用法：
//   1) 启动 server： A2A_GRPC_ADDR=127.0.0.1:50061 ./bin/a2a-gateway.exe
//   2) 跑 smoke：   ./bin/a2a_smoke.exe
//
// 退出码：0 = 全部 OK；非 0 = 失败（CI / 手动验收用）。
//
// 检查清单（12 项）：
//   1-5  Sprint 5：RegisterCard ×2 / Discover / SendMessage ok / SendMessage F_004 / Stream 3
//   6    生成 alice2/bob2 密钥对
//   7    RegisterCard alice2 带 ed25519 公钥 → accepted:true
//   8    SendMessage alice2→bob2 签过 → delivered:true
//   9    SendMessage alice2→bob2 翻 payload 重签 → F_007
//   10   SendMessage alice2→bob2 ts_ms=now-1h → F_008
//   11   RegisterCard auth["ed25519"]="!!notbase64" → gRPC InvalidArgument F_006
//   12   Stream alice2→bob2 2 条签过 + 1 条翻 sig → 第 3 条 Unauthenticated
package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	a2av1 "github.com/aicity/proto/gen/go/a2a/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// signCanonical 直接镜像 server a2asrv.canonicalEnvelope 字段顺序。
// 任何字段顺序改动必须双签 server + smoke —— 见 docs/06-A2A-canonical.md。
func signCanonical(priv ed25519.PrivateKey, m *a2av1.Message) string {
	env := struct {
		MessageID      string `json:"message_id"`
		FromAgentID    string `json:"from_agent_id"`
		ToAgentID      string `json:"to_agent_id"`
		ConversationID string `json:"conversation_id"`
		Type           string `json:"type"`
		PayloadB64     string `json:"payload_b64"`
		TsMs           int64  `json:"ts_ms"`
		TraceID        string `json:"trace_id"`
	}{
		MessageID:      m.GetMessageId(),
		FromAgentID:    m.GetFromAgentId(),
		ToAgentID:      m.GetToAgentId(),
		ConversationID: m.GetConversationId(),
		Type:           m.GetType(),
		PayloadB64:     base64.RawStdEncoding.EncodeToString(m.GetPayload()),
		TsMs:           m.GetTsMs(),
		TraceID:        m.GetTraceId(),
	}
	body, _ := json.Marshal(env)
	return base64.StdEncoding.EncodeToString(ed25519.Sign(priv, body))
}

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

	// === Sprint 5 原 5 项 ===

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
	if _, err := stream.Recv(); err != nil && err != io.EOF {
		fmt.Printf("[FAIL] Stream EOF: %v\n", err)
		os.Exit(17)
	}
	fmt.Printf("[OK]   Stream echo 3 messages\n")

	// === Sprint 5.5 新增 7 项 ===

	// 6) 生成 alice2/bob2 密钥对（bob2 不强制签，pub/priv 仅用于占位）
	alice2Pub, alice2Priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		fmt.Printf("[FAIL] GenerateKey alice2: %v\n", err)
		os.Exit(18)
	}
	fmt.Printf("[OK]   generated alice2 ed25519 keypair\n")

	// 7) RegisterCard alice2 带 ed25519 公钥 → accepted:true
	if _, err := c.RegisterCard(ctx, &a2av1.AgentCard{
		AgentId: "alice2", Name: "Alice2", Provider: "aicity",
		Auth: map[string]string{"ed25519": base64.StdEncoding.EncodeToString(alice2Pub)},
	}); err != nil {
		fmt.Printf("[FAIL] register alice2: %v\n", err)
		os.Exit(18)
	}
	if _, err := c.RegisterCard(ctx, &a2av1.AgentCard{
		AgentId: "bob2", Name: "Bob2", Provider: "aicity",
	}); err != nil {
		fmt.Printf("[FAIL] register bob2: %v\n", err)
		os.Exit(18)
	}
	fmt.Printf("[OK]   RegisterCard alice2 (with ed25519 pubkey) accepted=true\n")

	// 8) SendMessage alice2→bob2 签过 → delivered:true
	now := time.Now()
	mSigned := &a2av1.Message{
		MessageId: "m_signed_ok", FromAgentId: "alice2", ToAgentId: "bob2",
		Type: "request", Payload: []byte("signed hi"), TsMs: now.UnixMilli(),
	}
	mSigned.Signature = signCanonical(alice2Priv, mSigned)

	resp, err = c.SendMessage(ctx, mSigned)
	if err != nil {
		fmt.Printf("[FAIL] SendMessage signed: %v\n", err)
		os.Exit(19)
	}
	if !resp.GetDelivered() || resp.GetError() != "" {
		fmt.Printf("[FAIL] SendMessage signed delivered=%v err=%q\n", resp.GetDelivered(), resp.GetError())
		os.Exit(19)
	}
	fmt.Printf("[OK]   SendMessage alice2→bob2 (signed) delivered=true\n")

	// 9) SendMessage alice2→bob2 翻 payload 后重签 → F_007
	orig := &a2av1.Message{
		MessageId: "m_tamper", FromAgentId: "alice2", ToAgentId: "bob2",
		Type: "request", Payload: []byte("orig"), TsMs: now.UnixMilli(),
	}
	mTampered := &a2av1.Message{
		MessageId: orig.MessageId, FromAgentId: orig.FromAgentId, ToAgentId: orig.ToAgentId,
		Type: orig.Type, Payload: append([]byte{}, orig.Payload...), TsMs: orig.TsMs,
	}
	mTampered.Signature = signCanonical(alice2Priv, orig)
	mTampered.Payload[0] ^= 0x01

	resp, err = c.SendMessage(ctx, mTampered)
	if err != nil {
		fmt.Printf("[FAIL] SendMessage tampered: %v\n", err)
		os.Exit(20)
	}
	if resp.GetDelivered() {
		fmt.Printf("[FAIL] tampered should not be delivered\n")
		os.Exit(20)
	}
	if got := resp.GetError(); len(got) < 5 || got[:5] != "F_007" {
		fmt.Printf("[FAIL] tampered error want F_007 prefix, got %q\n", got)
		os.Exit(20)
	}
	fmt.Printf("[OK]   SendMessage alice2→bob2 (tampered) delivered=false error=F_007\n")

	// 10) SendMessage alice2→bob2 ts_ms=now-1h → F_008
	stale := time.Now().Add(-1 * time.Hour).UnixMilli()
	mStale := &a2av1.Message{
		MessageId: "m_stale", FromAgentId: "alice2", ToAgentId: "bob2",
		Type: "request", Payload: []byte("stale"), TsMs: stale,
	}
	mStale.Signature = signCanonical(alice2Priv, mStale)

	resp, err = c.SendMessage(ctx, mStale)
	if err != nil {
		fmt.Printf("[FAIL] SendMessage stale: %v\n", err)
		os.Exit(21)
	}
	if resp.GetDelivered() {
		fmt.Printf("[FAIL] stale should not be delivered\n")
		os.Exit(21)
	}
	if got := resp.GetError(); len(got) < 5 || got[:5] != "F_008" {
		fmt.Printf("[FAIL] stale error want F_008 prefix, got %q\n", got)
		os.Exit(21)
	}
	fmt.Printf("[OK]   SendMessage alice2→bob2 (stale ts) delivered=false error=F_008\n")

	// 11) RegisterCard auth["ed25519"]="!!notbase64" → gRPC InvalidArgument F_006
	_, err = c.RegisterCard(ctx, &a2av1.AgentCard{
		AgentId: "evil", Name: "Evil",
		Auth: map[string]string{"ed25519": "!!notbase64!!"},
	})
	if err == nil {
		fmt.Printf("[FAIL] invalid pubkey should return error\n")
		os.Exit(22)
	}
	if status.Code(err) != codes.InvalidArgument {
		fmt.Printf("[FAIL] invalid pubkey code want InvalidArgument got %s\n", status.Code(err))
		os.Exit(22)
	}
	if msg := status.Convert(err).Message(); len(msg) < 5 || msg[:5] != "F_006" {
		fmt.Printf("[FAIL] invalid pubkey msg want F_006 prefix, got %q\n", msg)
		os.Exit(22)
	}
	fmt.Printf("[OK]   RegisterCard invalid ed25519 pubkey → gRPC InvalidArgument F_006\n")

	// 12) Stream alice2→bob2 2 条签过 + 1 条翻 sig → 第 3 条 Unauthenticated
	stream2, err := c.Stream(ctx)
	if err != nil {
		fmt.Printf("[FAIL] Stream2 open: %v\n", err)
		os.Exit(23)
	}
	now2 := time.Now()
	for i := 1; i <= 2; i++ {
		m := &a2av1.Message{
			MessageId:   fmt.Sprintf("sm_ok_%d", i),
			FromAgentId: "alice2", ToAgentId: "bob2",
			Type:    "request",
			Payload: []byte(fmt.Sprintf("ok%d", i)),
			TsMs:    now2.UnixMilli(),
		}
		m.Signature = signCanonical(alice2Priv, m)
		if err := stream2.Send(m); err != nil {
			fmt.Printf("[FAIL] Stream2 send ok%d: %v\n", i, err)
			os.Exit(23)
		}
	}
	for i := 1; i <= 2; i++ {
		if _, err := stream2.Recv(); err != nil {
			fmt.Printf("[FAIL] Stream2 recv ok%d: %v\n", i, err)
			os.Exit(23)
		}
	}
	mBad := &a2av1.Message{
		MessageId: "sm_bad_3", FromAgentId: "alice2", ToAgentId: "bob2",
		Type: "request", Payload: []byte("bad"), TsMs: now2.UnixMilli(),
	}
	sigBytes, _ := base64.StdEncoding.DecodeString(signCanonical(alice2Priv, mBad))
	sigBytes[0] ^= 0x01
	mBad.Signature = base64.StdEncoding.EncodeToString(sigBytes)
	if err := stream2.Send(mBad); err != nil {
		fmt.Printf("[FAIL] Stream2 send bad3: %v\n", err)
		os.Exit(23)
	}
	_, err = stream2.Recv()
	if err == nil {
		fmt.Printf("[FAIL] Stream2 recv bad3 want error, got nil\n")
		os.Exit(23)
	}
	if status.Code(err) != codes.Unauthenticated {
		fmt.Printf("[FAIL] Stream2 bad3 code want Unauthenticated got %s\n", status.Code(err))
		os.Exit(23)
	}
	if msg := status.Convert(err).Message(); len(msg) < 5 || msg[:5] != "F_007" {
		fmt.Printf("[FAIL] Stream2 bad3 msg want F_007 prefix, got %q\n", msg)
		os.Exit(23)
	}
	fmt.Printf("[OK]   Stream alice2→bob2 (bad sig on 3rd) → Unauthenticated F_007\n")

	fmt.Printf("\n[OK] all 12 a2a_smoke checks passed against %s\n", addr)
}
