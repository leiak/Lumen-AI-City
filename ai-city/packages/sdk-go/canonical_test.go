// Canonical 字节序列单测（Sprint 7+）：
//   1) EmptyAllBlank       — 全空字段也合法
//   2) Deterministic       — 同 struct 重复 Marshal → 相同字节
//   3) FieldOrder          — message_id 必须出现在最前；trace_id 在最后
//   4) PayloadRawStd       — payload_b64 用 RawStdEncoding（无 padding）
//   5) PaddingBoundary     — RawStdEncoding 在边界字节数时不出现 padding
//   6) StableAcrossReorder — struct 字段声明顺序固定 → 不会因字段顺序变化漂移
//   7) SignMessage_Roundtrip — SignMessage 产生可被独立 ed25519.Verify 验证的签名
//
// 设计目标：以上 7 用例任何一项失败 → 立刻触发跨实现护栏（verifier 端 TestVerifier_CanonicalBytes_MatchesSDK）
// + smoke binary (a2a_smoke 12 项) 端到端验证。
package aicity

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"
)

// ---------- 1) EmptyAllBlank ----------

func TestCanonicalBytes_EmptyAllBlank(t *testing.T) {
	b := CanonicalBytes(Signable{})
	// 全空 struct marshal 后形如：
	//   {"message_id":"","from_agent_id":"","to_agent_id":"","conversation_id":"","type":"","payload_b64":"","ts_ms":0,"trace_id":""}
	got := string(b)
	if got == "" {
		t.Fatal("canonical empty all-blank should produce non-empty JSON object")
	}
	// 8 个字段 key 都得出现
	for _, key := range []string{
		`"message_id":""`,
		`"from_agent_id":""`,
		`"to_agent_id":""`,
		`"conversation_id":""`,
		`"type":""`,
		`"payload_b64":""`,
		`"ts_ms":0`,
		`"trace_id":""`,
	} {
		if !strings.Contains(got, key) {
			t.Errorf("missing key %q in canonical: %s", key, got)
		}
	}
	// signature 字段必须缺席
	if strings.Contains(got, "signature") {
		t.Errorf("canonical must not contain signature field, got %s", got)
	}
}

// ---------- 2) Deterministic ----------

func TestCanonicalBytes_Deterministic(t *testing.T) {
	s := Signable{
		MessageID:      "m-1",
		FromAgentID:    "alice",
		ToAgentID:      "bob",
		ConversationID: "conv-001",
		Type:           "request",
		PayloadB64:     base64.RawStdEncoding.EncodeToString([]byte("hello")),
		TsMs:           1736112000000,
		TraceID:        "trace-abc",
	}
	b1 := CanonicalBytes(s)
	b2 := CanonicalBytes(s)
	if string(b1) != string(b2) {
		t.Fatalf("canonical not deterministic:\nb1=%s\nb2=%s", b1, b2)
	}
}

// ---------- 3) FieldOrder ----------

func TestCanonicalBytes_FieldOrder(t *testing.T) {
	s := Signable{
		MessageID:      "m",
		FromAgentID:    "a",
		ToAgentID:      "b",
		ConversationID: "c",
		Type:           "request",
		PayloadB64:     "aGk", // "hi"
		TsMs:           12345,
		TraceID:        "t",
	}
	got := string(CanonicalBytes(s))
	// message_id 必须最前
	if !strings.HasPrefix(got, `{"message_id":"m"`) {
		t.Errorf("canonical prefix wrong (want message_id first): %s", got)
	}
	// trace_id 必须最后（在 } 之前）
	if !strings.HasSuffix(got, `"trace_id":"t"}`) {
		t.Errorf("canonical suffix wrong (want trace_id last): %s", got)
	}
	// 字段顺序：每个 key 的下标必须严格递增
	keys := []string{
		`"message_id"`,
		`"from_agent_id"`,
		`"to_agent_id"`,
		`"conversation_id"`,
		`"type"`,
		`"payload_b64"`,
		`"ts_ms"`,
		`"trace_id"`,
	}
	prev := -1
	for _, k := range keys {
		i := strings.Index(got, k)
		if i < 0 {
			t.Errorf("missing key %s in canonical: %s", k, got)
			continue
		}
		if i <= prev {
			t.Errorf("field %s out of order at idx=%d (prev=%d): %s", k, i, prev, got)
		}
		prev = i
	}
}

// ---------- 4) PayloadRawStd ----------

func TestCanonicalBytes_PayloadRawStd(t *testing.T) {
	// "hello" → base64.RawStdEncoding = "aGVsbG8"（无 padding）
	s := Signable{
		PayloadB64: base64.RawStdEncoding.EncodeToString([]byte("hello")),
	}
	got := string(CanonicalBytes(s))
	want := `"payload_b64":"aGVsbG8"`
	if !strings.Contains(got, want) {
		t.Errorf("canonical payload_b64 want %q in: %s", want, got)
	}
	// 确认没出现带 padding 的形态（"aGVsbG8="）
	if strings.Contains(got, `"payload_b64":"aGVsbG8="`) {
		t.Errorf("canonical must use RawStdEncoding (no padding), got: %s", got)
	}
}

// ---------- 5) PaddingBoundary ----------

func TestCanonicalBytes_PaddingBoundary(t *testing.T) {
	// 边界字节数：1 byte → "AA=="，2 bytes → "AAA"，3 bytes → "AAAA"
	// 期望 RawStdEncoding 都不带 padding
	cases := []struct {
		payload []byte
		wantB64 string
	}{
		{[]byte{0x00}, "AA"},                // 1 byte → 2 chars (raw)
		{[]byte{0x00, 0x00}, "AAA"},         // 2 bytes → 3 chars (raw)
		{[]byte{0x00, 0x00, 0x00}, "AAAA"},  // 3 bytes → 4 chars (raw)
		{[]byte{0x00, 0x00, 0x00, 0x00}, "AAAAAA"},  // 4 bytes → 6 chars (raw, no padding needed)
	}
	for _, c := range cases {
		raw := base64.RawStdEncoding.EncodeToString(c.payload)
		if raw != c.wantB64 {
			t.Errorf("RawStdEncoding(%v) = %q, want %q", c.payload, raw, c.wantB64)
		}
		s := Signable{PayloadB64: raw}
		got := string(CanonicalBytes(s))
		want := `"payload_b64":"` + c.wantB64 + `"`
		if !strings.Contains(got, want) {
			t.Errorf("canonical boundary payload=%v want %q in: %s", c.payload, want, got)
		}
	}
}

// ---------- 6) StableAcrossReorder ----------

// 单独跑多次调用验证 struct 字段顺序固定 → 不会因调用栈 / runtime 重排。
func TestCanonicalBytes_StableAcrossReorder(t *testing.T) {
	s := Signable{
		MessageID:      "stable",
		FromAgentID:    "x",
		ToAgentID:      "y",
		ConversationID: "z",
		Type:           "request",
		PayloadB64:     "cHg=", // wait this is wrong, fix below
		TsMs:           999,
		TraceID:        "tr",
	}
	// 用一个合法 RawStd payload（避免误以为 RawStd 与 StdEncoding 兼容）
	s.PayloadB64 = base64.RawStdEncoding.EncodeToString([]byte("xy"))

	// 多次调用，确保输出稳定
	for i := 0; i < 5; i++ {
		b := CanonicalBytes(s)
		if !strings.HasPrefix(string(b), `{"message_id":"stable"`) {
			t.Fatalf("iter %d: prefix wrong: %s", i, b)
		}
		if !strings.HasSuffix(string(b), `"trace_id":"tr"}`) {
			t.Fatalf("iter %d: suffix wrong: %s", i, b)
		}
	}
}

// ---------- 7) SignMessage_Roundtrip ----------

func TestSignMessage_Roundtrip(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	s := Signable{
		MessageID:      "roundtrip-1",
		FromAgentID:    "alice",
		ToAgentID:      "bob",
		ConversationID: "conv-rt",
		Type:           "request",
		PayloadB64:     base64.RawStdEncoding.EncodeToString([]byte("roundtrip payload")),
		TsMs:           1700000000000,
		TraceID:        "trace-rt",
	}

	sigStr, err := SignMessage(priv, s)
	if err != nil {
		t.Fatalf("SignMessage: %v", err)
	}
	sigBytes, err := base64.StdEncoding.DecodeString(sigStr)
	if err != nil {
		t.Fatalf("decode sig: %v", err)
	}
	if len(sigBytes) != ed25519.SignatureSize {
		t.Errorf("sig size = %d, want %d", len(sigBytes), ed25519.SignatureSize)
	}

	// 独立 ed25519.Verify（不依赖 SDK SignMessage 自身）
	if !ed25519.Verify(pub, CanonicalBytes(s), sigBytes) {
		t.Fatal("ed25519.Verify failed against CanonicalBytes")
	}
}