// Service 集成测试：用 bufconn 在进程内起 fake gRPC server 测 4 RPC。
//
// Sprint 5 旧测试（向后兼容）+ Sprint 5.5 新增 8 个签名/路由集成测。
// 模式参考 apps/api-gateway/internal/worldgrpc/client_test.go（Sprint 3.5）。
package a2asrv

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
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

// newSignedService 构造一个带 EchoAdapter fallback 的 Service（opt-in 兼容旧测试）。
func newSignedService() (*Service, *Dispatcher) {
	d := NewDispatcher()
	d.Register(EchoAdapter{})
	d.SetFallback(EchoAdapter{})
	return NewService(NewRegistry(), NewVerifier(5*time.Minute), d), d
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
	// 旧 opt-in：alice / bob 都不带 key
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

	for i, want := range msgs {
		got, err := stream.Recv()
		if err != nil {
			t.Fatalf("Recv[%d]: %v", i, err)
		}
		if got.GetFromAgentId() != want.GetToAgentId() {
			t.Errorf("msg %d: from want %q got %q (swap failed)", i, want.GetToAgentId(), got.GetFromAgentId())
		}
		if got.GetToAgentId() != want.GetFromAgentId() {
			t.Errorf("msg %d: to want %q got %q (swap failed)", i, want.GetFromAgentId(), got.GetToAgentId())
		}
		if got.GetType() != "event" {
			t.Errorf("msg %d: type want event got %q", i, got.GetType())
		}
		if string(got.GetPayload()) != string(want.GetPayload()) {
			t.Errorf("msg %d: payload want %q got %q", i, want.GetPayload(), got.GetPayload())
		}
		if got.GetSignature() != "" {
			t.Errorf("msg %d: signature should be cleared, got %q", i, got.GetSignature())
		}
	}
}

func TestService_Stream_ValidSignature_3Messages(t *testing.T) {
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
			MessageId: "sm" + string(rune('0'+i)),
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
	for i := 1; i <= 3; i++ {
		got, err := stream.Recv()
		if err != nil {
			t.Fatalf("Recv[%d]: %v", i, err)
		}
		if got.GetType() != "event" {
			t.Errorf("msg %d: type want event got %q", i, got.GetType())
		}
	}
}

func TestService_Stream_BadSignature_StreamCloses(t *testing.T) {
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

	// 第 1 条：签过 → ok
	m1 := &a2av1.Message{MessageId: "ok1", FromAgentId: "alice", ToAgentId: "bob",
		Type: "request", Payload: []byte("ok"), TsMs: now.UnixMilli()}
	m1.Signature = signFor(t, priv, m1)
	if err := stream.Send(m1); err != nil {
		t.Fatalf("Send m1: %v", err)
	}
	if _, err := stream.Recv(); err != nil {
		t.Fatalf("Recv m1: %v", err)
	}

	// 第 2 条：翻 signature 一字节
	m2 := &a2av1.Message{MessageId: "bad2", FromAgentId: "alice", ToAgentId: "bob",
		Type: "request", Payload: []byte("bad"), TsMs: now.UnixMilli()}
	sigBytes, _ := base64.StdEncoding.DecodeString(signFor(t, priv, m2))
	sigBytes[0] ^= 0x01
	m2.Signature = base64.StdEncoding.EncodeToString(sigBytes)
	if err := stream.Send(m2); err != nil {
		t.Fatalf("Send m2: %v", err)
	}
	_, err = stream.Recv()
	if err == nil {
		t.Fatal("Recv after bad sig want error, got nil")
	}
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("want codes.Unauthenticated got %s", status.Code(err))
	}
	if !strings.HasPrefix(status.Convert(err).Message(), "F_007:") {
		t.Errorf("msg want F_007: prefix got %q", status.Convert(err).Message())
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
