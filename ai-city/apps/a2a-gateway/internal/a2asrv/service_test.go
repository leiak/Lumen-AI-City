// Service 集成测试：用 bufconn 在进程内起 fake gRPC server 测 5 RPC。
//
// Sprint 5 旧测试（向后兼容）+ Sprint 5.5 签名/路由集成测
// + Sprint 8 ACL 投递门 / Discover cityFilter。
// 模式参考 apps/api-gateway/internal/worldgrpc/client_test.go（Sprint 3.5）。
package a2asrv

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	a2av1 "github.com/aicity/proto/gen/go/a2a/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

// EOFStr is the string representation of io.EOF in gRPC stream responses.
const EOFStr = "EOF"

// newBufconnClient 启动 bufconn server + 返回客户端 stub。
func newBufconnClient(t *testing.T, svc *Service) (a2av1.A2AGatewayClient, *Registry) {
	t.Helper()
	lis := bufconn.Listen(1024 * 64)
	srv := grpc.NewServer()
	a2av1.RegisterA2AGatewayServer(srv, svc)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(func() {
		srv.GracefulStop()
		_ = lis.Close()
	})

	dialer := func(context.Context, string) (net.Conn, error) { return lis.Dial() }
	conn, err := grpc.NewClient(
		"passthrough://bufnet",
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("grpc.NewClient: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return a2av1.NewA2AGatewayClient(conn), svc.reg
}

// newSignedService 构造一个带 InboxAdapter fallback 的 Service（opt-in 兼容旧测试）。
//
// Sprint 7 改动：
//   - EchoAdapter → InboxAdapter（nil store 静默返 success）
//   - NewService 新增 inbox 参数；旧测试用 nil
//
// Sprint 8 改动：
//   - NewService 新增 acl 参数；旧测试用 nil → 默认 allow，行为不变
func newSignedService() (*Service, *Dispatcher) {
	return newSignedServiceWithACL(nil)
}

// newSignedServiceWithACL 同上，但可注入 ACL（Sprint 8 ACL 用例用）。
func newSignedServiceWithACL(acl *ACL) (*Service, *Dispatcher) {
	d := NewDispatcher()
	inboxAdapter := NewInboxAdapter(nil)
	d.Register(inboxAdapter)
	d.SetFallback(inboxAdapter)
	return NewService(NewRegistry(), NewVerifier(5*time.Minute), d, nil, acl), d
}

// signFor 用 priv 对 m 做签，返 base64 字符串。
func signFor(t *testing.T, priv ed25519.PrivateKey, m *a2av1.Message) string {
	t.Helper()
	sig := ed25519.Sign(priv, canonicalBytes(m))
	return base64.StdEncoding.EncodeToString(sig)
}

// ---------- RegisterCard（旧） ----------

func TestService_RegisterCard_OK(t *testing.T) {
	svc, _ := newSignedService()
	c, _ := newBufconnClient(t, svc)

	resp, err := c.RegisterCard(context.Background(), &a2av1.AgentCard{
		AgentId:      "alice",
		Name:         "Alice",
		Capabilities: []string{"chat"},
	})
	if err != nil {
		t.Fatalf("RegisterCard: %v", err)
	}
	if !resp.GetAccepted() {
		t.Errorf("accepted=false")
	}
	if resp.GetCardId() != "alice" {
		t.Errorf("card_id want alice got %q", resp.GetCardId())
	}
}

func TestService_RegisterCard_Duplicate_Idempotent(t *testing.T) {
	svc, _ := newSignedService()
	c, reg := newBufconnClient(t, svc)

	card := &a2av1.AgentCard{AgentId: "alice", Name: "Alice v1", Capabilities: []string{"chat"}}
	if _, err := c.RegisterCard(context.Background(), card); err != nil {
		t.Fatalf("first register: %v", err)
	}
	card2 := &a2av1.AgentCard{AgentId: "alice", Name: "Alice v2", Capabilities: []string{"chat", "search"}}
	resp, err := c.RegisterCard(context.Background(), card2)
	if err != nil {
		t.Fatalf("dup register: %v", err)
	}
	if !resp.GetAccepted() {
		t.Errorf("dup want accepted=true (idempotent)")
	}
	got, _ := reg.Get("alice")
	if got.GetName() != "Alice v2" {
		t.Errorf("registry not overwritten: got %q", got.GetName())
	}
	if len(got.GetCapabilities()) != 2 {
		t.Errorf("caps want 2 got %d", len(got.GetCapabilities()))
	}
}

func TestService_RegisterCard_MissingFields_Returns_F001(t *testing.T) {
	svc, _ := newSignedService()
	c, _ := newBufconnClient(t, svc)

	cases := []*a2av1.AgentCard{
		{AgentId: "", Name: "X"},
		{AgentId: "x", Name: ""},
		nil,
	}
	for i, card := range cases {
		_, err := c.RegisterCard(context.Background(), card)
		if err == nil {
			t.Errorf("case %d: want error, got nil", i)
			continue
		}
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("case %d: code want InvalidArgument got %s", i, status.Code(err))
		}
		if !strings.HasPrefix(status.Convert(err).Message(), "F_001:") {
			t.Errorf("case %d: msg want F_001: prefix got %q", i, status.Convert(err).Message())
		}
	}
}

// ---------- Discover（旧） ----------

func TestService_Discover_ByCapability(t *testing.T) {
	svc, _ := newSignedService()
	c, _ := newBufconnClient(t, svc)
	ctx := context.Background()

	for _, id := range []struct {
		id  string
		cap []string
	}{
		{"alice", []string{"chat"}},
		{"bob", []string{"search", "chat"}},
		{"carol", []string{"search"}},
	} {
		_, err := c.RegisterCard(ctx, &a2av1.AgentCard{AgentId: id.id, Name: id.id, Capabilities: id.cap})
		if err != nil {
			t.Fatalf("register %s: %v", id.id, err)
		}
	}

	resp, err := c.Discover(ctx, &a2av1.DiscoverRequest{Capability: "chat"})
	if err != nil {
		t.Fatalf("Discover chat: %v", err)
	}
	if got := len(resp.GetCards()); got != 2 {
		t.Errorf("chat want 2 cards got %d", got)
	}

	resp, err = c.Discover(ctx, &a2av1.DiscoverRequest{Capability: "search"})
	if err != nil {
		t.Fatalf("Discover search: %v", err)
	}
	if got := len(resp.GetCards()); got != 2 {
		t.Errorf("search want 2 cards got %d", got)
	}

	resp, err = c.Discover(ctx, &a2av1.DiscoverRequest{Capability: "nope"})
	if err != nil {
		t.Fatalf("Discover nope: %v", err)
	}
	if got := len(resp.GetCards()); got != 0 {
		t.Errorf("nope want 0 got %d", got)
	}
}

func TestService_Discover_EmptyCapability_Returns_F003(t *testing.T) {
	svc, _ := newSignedService()
	c, _ := newBufconnClient(t, svc)

	_, err := c.Discover(context.Background(), &a2av1.DiscoverRequest{Capability: ""})
	if err == nil {
		t.Fatal("want error")
	}
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("code want InvalidArgument got %s", status.Code(err))
	}
	if !strings.HasPrefix(status.Convert(err).Message(), "F_003:") {
		t.Errorf("msg want F_003: prefix got %q", status.Convert(err).Message())
	}
}

// ---------- SendMessage（旧 + Sprint 5.5 增量） ----------

func TestService_SendMessage_Delivered(t *testing.T) {
	svc, _ := newSignedService()
	c, _ := newBufconnClient(t, svc)
	ctx := context.Background()

	for _, id := range []string{"alice", "bob"} {
		if _, err := c.RegisterCard(ctx, &a2av1.AgentCard{AgentId: id, Name: id}); err != nil {
			t.Fatalf("register %s: %v", id, err)
		}
	}

	resp, err := c.SendMessage(ctx, &a2av1.Message{
		MessageId:   "m1",
		FromAgentId: "alice",
		ToAgentId:   "bob",
		Type:        "request",
		Payload:     []byte("hello"),
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if !resp.GetDelivered() {
		t.Errorf("want delivered=true, got %v / err=%q", resp.GetDelivered(), resp.GetError())
	}
	if resp.GetError() != "" {
		t.Errorf("want error empty got %q", resp.GetError())
	}
}

func TestService_SendMessage_UnknownRecipient_Returns_F004(t *testing.T) {
	svc, _ := newSignedService()
	c, _ := newBufconnClient(t, svc)
	ctx := context.Background()

	if _, err := c.RegisterCard(ctx, &a2av1.AgentCard{AgentId: "alice", Name: "Alice"}); err != nil {
		t.Fatalf("register alice: %v", err)
	}

	resp, err := c.SendMessage(ctx, &a2av1.Message{
		MessageId:   "m1",
		FromAgentId: "alice",
		ToAgentId:   "ghost",
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if resp.GetDelivered() {
		t.Errorf("want delivered=false")
	}
	if !strings.HasPrefix(resp.GetError(), "F_004:") {
		t.Errorf("error want F_004: prefix got %q", resp.GetError())
	}
}

func TestService_SendMessage_UnknownSender_Returns_F005(t *testing.T) {
	svc, _ := newSignedService()
	c, _ := newBufconnClient(t, svc)
	ctx := context.Background()

	if _, err := c.RegisterCard(ctx, &a2av1.AgentCard{AgentId: "bob", Name: "Bob"}); err != nil {
		t.Fatalf("register bob: %v", err)
	}

	resp, err := c.SendMessage(ctx, &a2av1.Message{
		MessageId:   "m1",
		FromAgentId: "ghost",
		ToAgentId:   "bob",
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if resp.GetDelivered() {
		t.Errorf("want delivered=false")
	}
	if !strings.HasPrefix(resp.GetError(), "F_005:") {
		t.Errorf("error want F_005: prefix got %q", resp.GetError())
	}
}

// ---------- Sprint 5.5: ed25519 验签 + 路由 ----------

func TestService_SendMessage_ValidSignature_Delivered(t *testing.T) {
	svc, _ := newSignedService()
	c, _ := newBufconnClient(t, svc)
	ctx := context.Background()

	// alice 带 key（必签）；bob 不带 key（opt-in 放行）
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	if _, err := c.RegisterCard(ctx, &a2av1.AgentCard{
		AgentId: "alice", Name: "Alice",
		Auth: map[string]string{"ed25519": base64.StdEncoding.EncodeToString(pub)},
	}); err != nil {
		t.Fatalf("register alice: %v", err)
	}
	if _, err := c.RegisterCard(ctx, &a2av1.AgentCard{AgentId: "bob", Name: "Bob"}); err != nil {
		t.Fatalf("register bob: %v", err)
	}

	now := time.Now()
	m := &a2av1.Message{
		MessageId: "m_signed", FromAgentId: "alice", ToAgentId: "bob",
		Type: "request", Payload: []byte("signed"),
		TsMs: now.UnixMilli(),
	}
	m.Signature = signFor(t, priv, m)

	resp, err := c.SendMessage(ctx, m)
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if !resp.GetDelivered() || resp.GetError() != "" {
		t.Errorf("want delivered=true err=\"\", got %v / %q", resp.GetDelivered(), resp.GetError())
	}
}

func TestService_SendMessage_TamperedSignature_F007(t *testing.T) {
	svc, _ := newSignedService()
	c, _ := newBufconnClient(t, svc)
	ctx := context.Background()

	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	c.RegisterCard(ctx, &a2av1.AgentCard{
		AgentId: "alice", Name: "Alice",
		Auth: map[string]string{"ed25519": base64.StdEncoding.EncodeToString(pub)},
	})
	c.RegisterCard(ctx, &a2av1.AgentCard{AgentId: "bob", Name: "Bob"})

	now := time.Now()
	orig := &a2av1.Message{
		MessageId: "m_tamper", FromAgentId: "alice", ToAgentId: "bob",
		Type: "request", Payload: []byte("orig"), TsMs: now.UnixMilli(),
	}
	m := &a2av1.Message{
		MessageId: orig.MessageId, FromAgentId: orig.FromAgentId, ToAgentId: orig.ToAgentId,
		Type: orig.Type, Payload: append([]byte{}, orig.Payload...), TsMs: orig.TsMs,
	}
	m.Signature = signFor(t, priv, orig)
	m.Payload[0] ^= 0x01 // 翻转 1 字节

	resp, err := c.SendMessage(ctx, m)
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if resp.GetDelivered() {
		t.Errorf("want delivered=false")
	}
	if !strings.HasPrefix(resp.GetError(), "F_007:") {
		t.Errorf("want F_007 prefix, got %q", resp.GetError())
	}
}

func TestService_SendMessage_MissingSignature_F007(t *testing.T) {
	svc, _ := newSignedService()
	c, _ := newBufconnClient(t, svc)
	ctx := context.Background()

	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	c.RegisterCard(ctx, &a2av1.AgentCard{
		AgentId: "alice", Name: "Alice",
		Auth: map[string]string{"ed25519": base64.StdEncoding.EncodeToString(pub)},
	})
	c.RegisterCard(ctx, &a2av1.AgentCard{AgentId: "bob", Name: "Bob"})

	resp, err := c.SendMessage(ctx, &a2av1.Message{
		MessageId: "m_nosig", FromAgentId: "alice", ToAgentId: "bob",
		Type: "request", Payload: []byte("x"),
		// Signature 空 → F_007 required
		TsMs: time.Now().UnixMilli(),
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if resp.GetDelivered() {
		t.Errorf("want delivered=false")
	}
	if !strings.HasPrefix(resp.GetError(), "F_007:") {
		t.Errorf("want F_007 prefix, got %q", resp.GetError())
	}
}

func TestService_SendMessage_StaleTsMs_F008(t *testing.T) {
	svc, _ := newSignedService()
	c, _ := newBufconnClient(t, svc)
	ctx := context.Background()

	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	c.RegisterCard(ctx, &a2av1.AgentCard{
		AgentId: "alice", Name: "Alice",
		Auth: map[string]string{"ed25519": base64.StdEncoding.EncodeToString(pub)},
	})
	c.RegisterCard(ctx, &a2av1.AgentCard{AgentId: "bob", Name: "Bob"})

	stale := time.Now().Add(-1 * time.Hour).UnixMilli()
	m := &a2av1.Message{
		MessageId: "m_stale", FromAgentId: "alice", ToAgentId: "bob",
		Type: "request", Payload: []byte("x"), TsMs: stale,
	}
	m.Signature = signFor(t, priv, m)

	resp, err := c.SendMessage(ctx, m)
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if resp.GetDelivered() {
		t.Errorf("want delivered=false")
	}
	if !strings.HasPrefix(resp.GetError(), "F_008:") {
		t.Errorf("want F_008 prefix, got %q", resp.GetError())
	}
}

func TestService_RegisterCard_InvalidPubkey_F006(t *testing.T) {
	svc, _ := newSignedService()
	c, _ := newBufconnClient(t, svc)
	_, err := c.RegisterCard(context.Background(), &a2av1.AgentCard{
		AgentId: "alice", Name: "Alice",
		Auth: map[string]string{"ed25519": "!!notbase64!!"},
	})
	if err == nil {
		t.Fatal("want error")
	}
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("code want InvalidArgument got %s", status.Code(err))
	}
	if !strings.HasPrefix(status.Convert(err).Message(), "F_006:") {
		t.Errorf("msg want F_006: prefix got %q", status.Convert(err).Message())
	}
}

// ---------- Stream（旧 + Sprint 5.5 增量） ----------

func TestService_Stream_Echo(t *testing.T) {
	// Sprint 7：InboxAdapter 是 queue-only（返 nil reply），Stream 无 swap 回传。
	// 验证：3 条消息 send 成功 + CloseSend 后 Recv 直接 EOF。
	svc, _ := newSignedService()
	c, _ := newBufconnClient(t, svc)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := c.Stream(ctx)
	if err != nil {
		t.Fatalf("Stream open: %v", err)
	}

	msgs := []*a2av1.Message{
		{MessageId: "m1", FromAgentId: "alice", ToAgentId: "bob", Type: "request", Payload: []byte("ping1"), TsMs: 100, TraceId: "t1"},
		{MessageId: "m2", FromAgentId: "alice", ToAgentId: "bob", Type: "request", Payload: []byte("ping2"), TsMs: 200, TraceId: "t2"},
		{MessageId: "m3", FromAgentId: "alice", ToAgentId: "bob", Type: "request", Payload: []byte("ping3"), TsMs: 300, TraceId: "t3"},
	}
	// 注册
	if _, err := c.RegisterCard(ctx, &a2av1.AgentCard{AgentId: "alice", Name: "alice"}); err != nil {
		t.Fatalf("register alice: %v", err)
	}
	if _, err := c.RegisterCard(ctx, &a2av1.AgentCard{AgentId: "bob", Name: "bob"}); err != nil {
		t.Fatalf("register bob: %v", err)
	}
	for _, m := range msgs {
		if err := stream.Send(m); err != nil {
			t.Fatalf("Send %s: %v", m.GetMessageId(), err)
		}
	}
	if err := stream.CloseSend(); err != nil {
		t.Fatalf("CloseSend: %v", err)
	}

	// InboxAdapter 返 nil reply → Stream 无回传 → Recv 直接 EOF
	_, err = stream.Recv()
	if err == nil {
		t.Fatal("want EOF after queue-only Stream")
	}
	if !strings.Contains(err.Error(), "EOF") && err != io.EOF {
		// gRPC stream EOF 可能用 io.EOF 或 "EOF" 字符串
		if err.Error() != EOFStr {
			t.Errorf("want EOF, got %v", err)
		}
	}
}

func TestService_Stream_ValidSignature_3Messages(t *testing.T) {
	// Sprint 7：3 条签名消息 + CloseSend → EOF（queue-only，无 echo 回传）
	svc, _ := newSignedService()
	c, _ := newBufconnClient(t, svc)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	if _, err := c.RegisterCard(ctx, &a2av1.AgentCard{
		AgentId: "alice", Name: "alice",
		Auth: map[string]string{"ed25519": base64.StdEncoding.EncodeToString(pub)},
	}); err != nil {
		t.Fatalf("register alice: %v", err)
	}
	if _, err := c.RegisterCard(ctx, &a2av1.AgentCard{AgentId: "bob", Name: "bob"}); err != nil {
		t.Fatalf("register bob: %v", err)
	}

	stream, err := c.Stream(ctx)
	if err != nil {
		t.Fatalf("Stream open: %v", err)
	}
	now := time.Now()
	for i := 1; i <= 3; i++ {
		m := &a2av1.Message{
			MessageId:   "sm" + string(rune('0'+i)),
			FromAgentId: "alice", ToAgentId: "bob",
			Type: "request", Payload: []byte("p" + string(rune('0'+i))),
			TsMs: now.UnixMilli(),
		}
		m.Signature = signFor(t, priv, m)
		if err := stream.Send(m); err != nil {
			t.Fatalf("Send[%d]: %v", i, err)
		}
	}
	if err := stream.CloseSend(); err != nil {
		t.Fatalf("CloseSend: %v", err)
	}
	// 3 条均被 InboxAdapter 接受 → Recv 返 EOF
	_, err = stream.Recv()
	if err == nil {
		t.Fatal("want EOF after queue-only Stream")
	}
	if err != io.EOF && err.Error() != EOFStr {
		t.Errorf("want EOF, got %v", err)
	}
}

func TestService_Stream_BadSignature_StreamCloses(t *testing.T) {
	// Sprint 7：第 1 条签名消息 → InboxAdapter 接受但返 nil reply（queue-only）。
	// 第 2 条翻 signature → Recv 返 Unauthenticated F_007（验签在 stream handler）。
	svc, _ := newSignedService()
	c, _ := newBufconnClient(t, svc)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	c.RegisterCard(ctx, &a2av1.AgentCard{
		AgentId: "alice", Name: "alice",
		Auth: map[string]string{"ed25519": base64.StdEncoding.EncodeToString(pub)},
	})
	c.RegisterCard(ctx, &a2av1.AgentCard{AgentId: "bob", Name: "bob"})

	stream, err := c.Stream(ctx)
	if err != nil {
		t.Fatalf("Stream open: %v", err)
	}
	now := time.Now()

	// 第 1 条：签过 → InboxAdapter 接受（nil reply），不报错
	m1 := &a2av1.Message{MessageId: "ok1", FromAgentId: "alice", ToAgentId: "bob",
		Type: "request", Payload: []byte("ok"), TsMs: now.UnixMilli()}
	m1.Signature = signFor(t, priv, m1)
	if err := stream.Send(m1); err != nil {
		t.Fatalf("Send m1: %v", err)
	}

	// 第 2 条：翻 signature 一字节 → 验签失败 → server 关闭流
	m2 := &a2av1.Message{MessageId: "bad2", FromAgentId: "alice", ToAgentId: "bob",
		Type: "request", Payload: []byte("bad"), TsMs: now.UnixMilli()}
	sigBytes, _ := base64.StdEncoding.DecodeString(signFor(t, priv, m2))
	sigBytes[0] ^= 0x01
	m2.Signature = base64.StdEncoding.EncodeToString(sigBytes)
	if err := stream.Send(m2); err != nil {
		t.Fatalf("Send m2: %v", err)
	}
	// 第 2 条 send 后 server 端验签失败 → 关流；下次 Recv 返 error
	// 注：InboxAdapter nil reply 也算 stream "end" for client side，
	//     但 Send 本身不会失败（消息进了 server 才知道验签失败）。
	//     需要 drain 一下才看到 error。
	for {
		_, recvErr := stream.Recv()
		if recvErr != nil {
			if status.Code(recvErr) != codes.Unauthenticated {
				t.Errorf("want codes.Unauthenticated got %s", status.Code(recvErr))
			}
			if !strings.HasPrefix(status.Convert(recvErr).Message(), "F_007:") {
				t.Errorf("msg want F_007: prefix got %q", status.Convert(recvErr).Message())
			}
			break
		}
	}
}

func TestService_Stream_StaleTs_StreamCloses(t *testing.T) {
	svc, _ := newSignedService()
	c, _ := newBufconnClient(t, svc)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	c.RegisterCard(ctx, &a2av1.AgentCard{
		AgentId: "alice", Name: "alice",
		Auth: map[string]string{"ed25519": base64.StdEncoding.EncodeToString(pub)},
	})
	c.RegisterCard(ctx, &a2av1.AgentCard{AgentId: "bob", Name: "bob"})

	stream, err := c.Stream(ctx)
	if err != nil {
		t.Fatalf("Stream open: %v", err)
	}
	stale := time.Now().Add(-2 * time.Hour).UnixMilli()
	m := &a2av1.Message{MessageId: "stale1", FromAgentId: "alice", ToAgentId: "bob",
		Type: "request", Payload: []byte("old"), TsMs: stale}
	m.Signature = signFor(t, priv, m)
	if err := stream.Send(m); err != nil {
		t.Fatalf("Send: %v", err)
	}
	_, err = stream.Recv()
	if err == nil {
		t.Fatal("Recv after stale ts want error, got nil")
	}
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("want codes.Unauthenticated got %s", status.Code(err))
	}
	if !strings.HasPrefix(status.Convert(err).Message(), "F_008:") {
		t.Errorf("msg want F_008: prefix got %q", status.Convert(err).Message())
	}
}

// ---------- Sprint 8：ACL 投递门 ----------

// registerSignedPair 注册 alice（带 key，city=beijing）+ bob（无 key，city=shanghai），
// 返回 alice 的私钥用于签名。
func registerSignedPair(t *testing.T, c a2av1.A2AGatewayClient) ed25519.PrivateKey {
	t.Helper()
	ctx := context.Background()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	if _, err := c.RegisterCard(ctx, &a2av1.AgentCard{
		AgentId: "alice", Name: "Alice", CityId: "beijing",
		Auth: map[string]string{"ed25519": base64.StdEncoding.EncodeToString(pub)},
	}); err != nil {
		t.Fatalf("register alice: %v", err)
	}
	if _, err := c.RegisterCard(ctx, &a2av1.AgentCard{
		AgentId: "bob", Name: "Bob", CityId: "shanghai",
	}); err != nil {
		t.Fatalf("register bob: %v", err)
	}
	return priv
}

// signedMsg 造一条 alice→bob 的已签名消息。
func signedMsg(t *testing.T, priv ed25519.PrivateKey, id string) *a2av1.Message {
	t.Helper()
	m := &a2av1.Message{
		MessageId: id, FromAgentId: "alice", ToAgentId: "bob",
		Type: "request", Payload: []byte("hi"),
		TsMs: time.Now().UnixMilli(),
	}
	m.Signature = signFor(t, priv, m)
	return m
}

// deny 命中 → delivered:false + F_016（走 MessageResponse.Error，不是 gRPC error）
func TestService_SendMessage_ACL_Deny(t *testing.T) {
	m := &mockDenyChecker{denied: true}
	svc, _ := newSignedServiceWithACL(&ACL{store: m})
	c, _ := newBufconnClient(t, svc)

	priv := registerSignedPair(t, c)
	resp, err := c.SendMessage(context.Background(), signedMsg(t, priv, "m_acl_deny"))
	if err != nil {
		t.Fatalf("SendMessage transport error: %v", err)
	}
	if resp.GetDelivered() {
		t.Error("delivered = true, want false (ACL deny)")
	}
	if !strings.HasPrefix(resp.GetError(), "F_016:") {
		t.Errorf("error = %q, want F_016: prefix", resp.GetError())
	}
	// ACL 查的是 sender.agent_id × recipient.city_id
	if m.gotAgentID != "alice" || m.gotPeerCity != "shanghai" {
		t.Errorf("acl queried (%q, %q), want (alice, shanghai)", m.gotAgentID, m.gotPeerCity)
	}
}

// 无 deny 策略 → 正常投递（默认 allow 不改变既有行为）
func TestService_SendMessage_ACL_DefaultAllow(t *testing.T) {
	m := &mockDenyChecker{denied: false}
	svc, _ := newSignedServiceWithACL(&ACL{store: m})
	c, _ := newBufconnClient(t, svc)

	priv := registerSignedPair(t, c)
	resp, err := c.SendMessage(context.Background(), signedMsg(t, priv, "m_acl_allow"))
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if !resp.GetDelivered() || resp.GetError() != "" {
		t.Errorf("want delivered=true err=\"\", got %v / %q", resp.GetDelivered(), resp.GetError())
	}
	if m.calls != 1 {
		t.Errorf("acl calls = %d, want 1", m.calls)
	}
}

// ACL 后端故障 → fail-closed，拒投而非放行
func TestService_SendMessage_ACL_StoreError_FailsClosed(t *testing.T) {
	m := &mockDenyChecker{err: errors.New("pg down")}
	svc, _ := newSignedServiceWithACL(&ACL{store: m})
	c, _ := newBufconnClient(t, svc)

	priv := registerSignedPair(t, c)
	resp, err := c.SendMessage(context.Background(), signedMsg(t, priv, "m_acl_err"))
	if err != nil {
		t.Fatalf("SendMessage transport error: %v", err)
	}
	if resp.GetDelivered() {
		t.Error("delivered = true on ACL backend failure, want false (fail-closed)")
	}
	if !strings.HasPrefix(resp.GetError(), "F_016:") {
		t.Errorf("error = %q, want F_016: prefix", resp.GetError())
	}
}

// 验签失败时不应该走到 ACL —— 先认证再授权
func TestService_SendMessage_ACL_NotConsultedOnBadSignature(t *testing.T) {
	m := &mockDenyChecker{denied: true}
	svc, _ := newSignedServiceWithACL(&ACL{store: m})
	c, _ := newBufconnClient(t, svc)

	priv := registerSignedPair(t, c)
	msg := signedMsg(t, priv, "m_bad_sig")
	msg.Payload = []byte("tampered") // 签名对不上了

	resp, err := c.SendMessage(context.Background(), msg)
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if !strings.HasPrefix(resp.GetError(), "F_007:") {
		t.Errorf("error = %q, want F_007: (signature checked before ACL)", resp.GetError())
	}
	if m.calls != 0 {
		t.Errorf("acl consulted %d times on bad signature, want 0", m.calls)
	}
}

// Stream 路径：ACL 拒绝 → PermissionDenied + 关流
func TestService_Stream_ACL_Deny_PermissionDenied(t *testing.T) {
	m := &mockDenyChecker{denied: true}
	svc, _ := newSignedServiceWithACL(&ACL{store: m})
	c, _ := newBufconnClient(t, svc)
	ctx := context.Background()

	priv := registerSignedPair(t, c)
	stream, err := c.Stream(ctx)
	if err != nil {
		t.Fatalf("Stream open: %v", err)
	}
	if err := stream.Send(signedMsg(t, priv, "m_stream_acl")); err != nil {
		t.Fatalf("Send: %v", err)
	}
	_, err = stream.Recv()
	if err == nil {
		t.Fatal("Recv after ACL deny want error, got nil")
	}
	// 身份已验过，是授权失败 → PermissionDenied（区别于 F_007/F_008 的 Unauthenticated）
	if status.Code(err) != codes.PermissionDenied {
		t.Errorf("want codes.PermissionDenied got %s", status.Code(err))
	}
	if !strings.HasPrefix(status.Convert(err).Message(), "F_016:") {
		t.Errorf("msg want F_016: prefix got %q", status.Convert(err).Message())
	}
}

// nil ACL（既有构造路径）→ 投递不受影响
func TestService_SendMessage_NilACL_Delivers(t *testing.T) {
	svc, _ := newSignedService() // acl = nil
	c, _ := newBufconnClient(t, svc)

	priv := registerSignedPair(t, c)
	resp, err := c.SendMessage(context.Background(), signedMsg(t, priv, "m_nil_acl"))
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if !resp.GetDelivered() || resp.GetError() != "" {
		t.Errorf("want delivered=true err=\"\", got %v / %q", resp.GetDelivered(), resp.GetError())
	}
}

// Discover 的 city_filter 经 Service 透传到 Registry（Sprint 8 端到端）
func TestService_Discover_CityFilter(t *testing.T) {
	svc, _ := newSignedService()
	c, _ := newBufconnClient(t, svc)
	ctx := context.Background()

	registerSignedPair(t, c) // alice@beijing, bob@shanghai —— 但都没 capabilities
	if _, err := c.RegisterCard(ctx, &a2av1.AgentCard{
		AgentId: "alice", Name: "Alice", CityId: "beijing", Capabilities: []string{"chat"},
	}); err != nil {
		t.Fatalf("register alice: %v", err)
	}
	if _, err := c.RegisterCard(ctx, &a2av1.AgentCard{
		AgentId: "bob", Name: "Bob", CityId: "shanghai", Capabilities: []string{"chat"},
	}); err != nil {
		t.Fatalf("register bob: %v", err)
	}

	all, err := c.Discover(ctx, &a2av1.DiscoverRequest{Capability: "chat"})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(all.GetCards()) != 2 {
		t.Errorf("no filter: want 2 cards got %d", len(all.GetCards()))
	}

	bj, err := c.Discover(ctx, &a2av1.DiscoverRequest{Capability: "chat", CityFilter: "beijing"})
	if err != nil {
		t.Fatalf("Discover(beijing): %v", err)
	}
	if len(bj.GetCards()) != 1 {
		t.Fatalf("city=beijing: want 1 card got %d", len(bj.GetCards()))
	}
	if bj.GetCards()[0].GetAgentId() != "alice" {
		t.Errorf("got %q, want alice", bj.GetCards()[0].GetAgentId())
	}
	// city_id 经 proto 往返
	if bj.GetCards()[0].GetCityId() != "beijing" {
		t.Errorf("city_id = %q, want beijing", bj.GetCards()[0].GetCityId())
	}
}
