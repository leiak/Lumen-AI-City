// Verifier 单元测试（Sprint 5.5）—— 14 用例：
//   公钥解析 OK / 坏 base64 / 错长度
//   round-trip / 单字节翻转 / 错 key
//   缺签强制 / opt-in 缺签放行
//   ts_ms future / stale / within
//   canonical deterministic / canonical 零化 signature
//   跨实现：server canonicalBytes byte-equal SDK CanonicalBytes + 真实验签回路
package a2asrv

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	aicity "github.com/aicity/sdk-go"
	a2av1 "github.com/aicity/proto/gen/go/a2a/v1"
)

// signCanonical 用给定 priv 对 m 的 canonical bytes 签；返 base64(std)。
// 与 verifier.canonicalBytes 共用契约 —— 测试同时验证两者一致。
func signCanonical(t *testing.T, priv ed25519.PrivateKey, m *a2av1.Message) string {
	t.Helper()
	sig := ed25519.Sign(priv, canonicalBytes(m))
	return base64.StdEncoding.EncodeToString(sig)
}

func newKeyPair(t *testing.T) (ed25519.PrivateKey, ed25519.PublicKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	return priv, pub
}

func signerCard(agentID string, pub ed25519.PublicKey) *a2av1.AgentCard {
	return &a2av1.AgentCard{
		AgentId: agentID,
		Name:    agentID,
		Auth:    map[string]string{"ed25519": base64.StdEncoding.EncodeToString(pub)},
	}
}

// ---------- ParsePublicKey ----------

func TestVerifier_ParsePublicKey_OK(t *testing.T) {
	_, pub := newKeyPair(t)
	v := NewVerifier(5 * time.Minute)
	got, err := v.ParsePublicKey(base64.StdEncoding.EncodeToString(pub))
	if err != nil {
		t.Fatalf("ParsePublicKey: %v", err)
	}
	if !got.Equal(pub) {
		t.Errorf("pub mismatch")
	}
}

func TestVerifier_ParsePublicKey_BadBase64(t *testing.T) {
	v := NewVerifier(5 * time.Minute)
	_, err := v.ParsePublicKey("!!!not-base64!!!")
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.HasPrefix(err.Error(), "F_007:") {
		t.Errorf("want F_007 prefix, got %q", err.Error())
	}
}

func TestVerifier_ParsePublicKey_WrongLength(t *testing.T) {
	v := NewVerifier(5 * time.Minute)
	short := base64.StdEncoding.EncodeToString([]byte{0x01, 0x02, 0x03})
	_, err := v.ParsePublicKey(short)
	if err == nil {
		t.Fatal("want error for wrong-length key")
	}
	if !strings.HasPrefix(err.Error(), "F_007:") {
		t.Errorf("want F_007 prefix, got %q", err.Error())
	}
}

// ---------- Verify ----------

func TestVerifier_Verify_RoundTrip_OK(t *testing.T) {
	priv, pub := newKeyPair(t)
	sender := signerCard("alice", pub)
	v := NewVerifier(5 * time.Minute)

	m := &a2av1.Message{
		MessageId: "m1", FromAgentId: "alice", ToAgentId: "bob",
		Type: "request", Payload: []byte("hello"), TsMs: time.Now().UnixMilli(),
	}
	m.Signature = signCanonical(t, priv, m)

	if err := v.Verify(sender, m, time.Now()); err != nil {
		t.Errorf("Verify want nil, got %v", err)
	}
}

func TestVerifier_Verify_SingleBitFlip_Fails(t *testing.T) {
	priv, pub := newKeyPair(t)
	sender := signerCard("alice", pub)
	v := NewVerifier(5 * time.Minute)

	orig := &a2av1.Message{
		MessageId: "m1", FromAgentId: "alice", ToAgentId: "bob",
		Type: "request", Payload: []byte("hello"), TsMs: time.Now().UnixMilli(),
	}
	m := &a2av1.Message{
		MessageId: orig.MessageId, FromAgentId: orig.FromAgentId, ToAgentId: orig.ToAgentId,
		Type: orig.Type, Payload: append([]byte{}, orig.Payload...), TsMs: orig.TsMs,
	}
	m.Signature = signCanonical(t, priv, orig)

	// 翻转 payload 一字节
	m.Payload[0] ^= 0x01

	err := v.Verify(sender, m, time.Now())
	if err == nil {
		t.Fatal("want error after flip")
	}
	if !strings.HasPrefix(err.Error(), "F_007:signature mismatch") {
		t.Errorf("want F_007:signature mismatch, got %q", err.Error())
	}
}

func TestVerifier_Verify_WrongKey_Fails(t *testing.T) {
	priv1, _ := newKeyPair(t)
	_, pub2 := newKeyPair(t) // 攻击者用别的 key 注册
	sender := signerCard("alice", pub2)
	v := NewVerifier(5 * time.Minute)

	m := &a2av1.Message{
		MessageId: "m1", FromAgentId: "alice", ToAgentId: "bob",
		Type: "request", Payload: []byte("hi"), TsMs: time.Now().UnixMilli(),
	}
	m.Signature = signCanonical(t, priv1, m) // alice 的私钥签的，但 sender 用 pub2

	err := v.Verify(sender, m, time.Now())
	if err == nil {
		t.Fatal("want mismatch")
	}
	if !strings.HasPrefix(err.Error(), "F_007:") {
		t.Errorf("want F_007 prefix, got %q", err.Error())
	}
}

func TestVerifier_Verify_NilSender_F005(t *testing.T) {
	v := NewVerifier(5 * time.Minute)
	err := v.Verify(nil, &a2av1.Message{}, time.Now())
	if err == nil || !strings.HasPrefix(err.Error(), "F_005:") {
		t.Errorf("want F_005, got %v", err)
	}
}

func TestVerifier_Verify_OptIn_NoKey_PassesWithoutSig(t *testing.T) {
	sender := &a2av1.AgentCard{AgentId: "alice", Name: "alice"} // 无 auth
	v := NewVerifier(5 * time.Minute)
	if err := v.Verify(sender, &a2av1.Message{}, time.Now()); err != nil {
		t.Errorf("opt-in want pass, got %v", err)
	}
}

func TestVerifier_Verify_OptIn_AuthNil_PassesWithoutSig(t *testing.T) {
	sender := &a2av1.AgentCard{AgentId: "alice", Name: "alice", Auth: nil}
	v := NewVerifier(5 * time.Minute)
	if err := v.Verify(sender, &a2av1.Message{}, time.Now()); err != nil {
		t.Errorf("opt-in (nil auth) want pass, got %v", err)
	}
}

func TestVerifier_Verify_HasKey_MissingSig_F007Required(t *testing.T) {
	_, pub := newKeyPair(t)
	sender := signerCard("alice", pub)
	v := NewVerifier(5 * time.Minute)
	m := &a2av1.Message{FromAgentId: "alice", ToAgentId: "bob", Type: "request"}
	err := v.Verify(sender, m, time.Now())
	if err == nil || !strings.HasPrefix(err.Error(), "F_007:signature required") {
		t.Errorf("want F_007:signature required, got %v", err)
	}
}

func TestVerifier_Verify_HasKey_BadBase64Sig_F007Decode(t *testing.T) {
	_, pub := newKeyPair(t)
	sender := signerCard("alice", pub)
	v := NewVerifier(5 * time.Minute)
	m := &a2av1.Message{
		FromAgentId: "alice", ToAgentId: "bob", Type: "request",
		Signature: "!!!not-base64!!!", TsMs: time.Now().UnixMilli(),
	}
	err := v.Verify(sender, m, time.Now())
	if err == nil || !strings.HasPrefix(err.Error(), "F_007:") {
		t.Errorf("want F_007, got %v", err)
	}
}

// ---------- ReplayWindow ----------

func TestVerifier_Verify_StaleTsMs_F008(t *testing.T) {
	priv, pub := newKeyPair(t)
	sender := signerCard("alice", pub)
	v := NewVerifier(5 * time.Minute)

	now := time.Now()
	staleMs := now.Add(-1 * time.Hour).UnixMilli()
	m := &a2av1.Message{
		MessageId: "m1", FromAgentId: "alice", ToAgentId: "bob",
		Type: "request", Payload: []byte("x"), TsMs: staleMs,
	}
	m.Signature = signCanonical(t, priv, m)

	err := v.Verify(sender, m, now)
	if err == nil || !strings.HasPrefix(err.Error(), "F_008:") {
		t.Errorf("want F_008, got %v", err)
	}
}

func TestVerifier_Verify_FutureTsMs_F008(t *testing.T) {
	priv, pub := newKeyPair(t)
	sender := signerCard("alice", pub)
	v := NewVerifier(5 * time.Minute)

	now := time.Now()
	futureMs := now.Add(1 * time.Hour).UnixMilli()
	m := &a2av1.Message{
		MessageId: "m1", FromAgentId: "alice", ToAgentId: "bob",
		Type: "request", Payload: []byte("x"), TsMs: futureMs,
	}
	m.Signature = signCanonical(t, priv, m)

	err := v.Verify(sender, m, now)
	if err == nil || !strings.HasPrefix(err.Error(), "F_008:") {
		t.Errorf("want F_008, got %v", err)
	}
}

func TestVerifier_Verify_WithinWindow_OK(t *testing.T) {
	priv, pub := newKeyPair(t)
	sender := signerCard("alice", pub)
	v := NewVerifier(5 * time.Minute)

	now := time.Now()
	m := &a2av1.Message{
		MessageId: "m1", FromAgentId: "alice", ToAgentId: "bob",
		Type: "request", Payload: []byte("x"),
		TsMs: now.Add(-30 * time.Second).UnixMilli(),
	}
	m.Signature = signCanonical(t, priv, m)

	if err := v.Verify(sender, m, now); err != nil {
		t.Errorf("want within-window pass, got %v", err)
	}
}

// ---------- canonical ----------

func TestVerifier_CanonicalBytes_Deterministic(t *testing.T) {
	m := &a2av1.Message{
		MessageId: "m", FromAgentId: "a", ToAgentId: "b",
		Type: "request", Payload: []byte("p"), TsMs: 12345,
		TraceId: "t",
	}
	b1 := canonicalBytes(m)
	b2 := canonicalBytes(m)
	if string(b1) != string(b2) {
		t.Errorf("canonical not deterministic")
	}
	// 字段顺序：message_id 必须在最前
	if !strings.HasPrefix(string(b1), `{"message_id":"m"`) {
		t.Errorf("canonical prefix wrong: %s", b1)
	}
	// payload 必须 RawStdEncoding（无 padding）
	wantPayload := base64.RawStdEncoding.EncodeToString([]byte("p"))
	if !strings.Contains(string(b1), `"payload_b64":"`+wantPayload+`"`) {
		t.Errorf("canonical payload_b64 wrong: %s", b1)
	}
}

func TestVerifier_CanonicalBytes_ZeroesSignature(t *testing.T) {
	m := &a2av1.Message{
		MessageId: "m", FromAgentId: "a", ToAgentId: "b",
		Type: "request", Payload: []byte("p"), TsMs: 1,
		Signature: "this-should-be-ignored",
	}
	b := canonicalBytes(m)
	if strings.Contains(string(b), "signature") {
		t.Errorf("canonical must not contain signature field, got %s", b)
	}
	if strings.Contains(string(b), "this-should-be-ignored") {
		t.Errorf("canonical must not contain signature value, got %s", b)
	}
}

// ---------- cross-impl 护栏（Sprint 7+）----------

// TestVerifier_CanonicalBytes_MatchesSDK 双向护栏：
//   1) server canonicalBytes 输出 byte-equal SDK CanonicalBytes 输出
//   2) 用 SDK SignMessage 签的 m，server Verify 必须通过（真实验签回路）
//
// 任何一边漂移（字段顺序 / tag / payload 编码）→ 立刻 fail，
// 同时 docs/06-A2A-canonical.md §五 必须双签 + 走 BREAKING 协议。
func TestVerifier_CanonicalBytes_MatchesSDK(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	s := aicity.Signable{
		MessageID:      "m-cross-1",
		FromAgentID:    "alice",
		ToAgentID:      "bob",
		ConversationID: "conv-cross",
		Type:           "request",
		PayloadB64:     base64.RawStdEncoding.EncodeToString([]byte("cross-impl payload")),
		TsMs:           1736112000000,
		TraceID:        "trace-cross",
	}

	// 把 Signable 翻成 proto Message（用于 server 端 canonicalBytes）
	m := msgFromSignable(s)

	// 1) byte-equal 断言
	serverBytes := canonicalBytes(m)
	sdkBytes := aicity.CanonicalBytes(s)
	if string(serverBytes) != string(sdkBytes) {
		t.Fatalf("server/sdk canonical bytes differ:\nserver=%s\nsdk   =%s", serverBytes, sdkBytes)
	}

	// 2) 真实验签回路：SDK SignMessage → server Verify
	sigStr, err := aicity.SignMessage(priv, s)
	if err != nil {
		t.Fatalf("SignMessage: %v", err)
	}
	m.Signature = sigStr

	sender := &a2av1.AgentCard{
		AgentId: "alice",
		Name:    "alice",
		Auth:    map[string]string{"ed25519": base64.StdEncoding.EncodeToString(pub)},
	}
	v := NewVerifier(5 * time.Minute)
	if err := v.Verify(sender, m, time.UnixMilli(s.TsMs)); err != nil {
		t.Fatalf("cross-impl verify failed: %v", err)
	}
}

// msgFromSignable 把 aicity.Signable 翻成 a2av1.Message（测试用辅助）。
func msgFromSignable(s aicity.Signable) *a2av1.Message {
	payload, _ := base64.RawStdEncoding.DecodeString(s.PayloadB64)
	return &a2av1.Message{
		MessageId:      s.MessageID,
		FromAgentId:    s.FromAgentID,
		ToAgentId:      s.ToAgentID,
		ConversationId: s.ConversationID,
		Type:           s.Type,
		Payload:        payload,
		TsMs:           s.TsMs,
		TraceId:        s.TraceID,
	}
}
