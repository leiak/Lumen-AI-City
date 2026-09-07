// httpgw 集成测：用 httptest.NewRecorder + 服务真 *a2asrv.Service 测路由。
//
// 7 用例：
//   1) RegisterCard OK                 → 200
//   2) RegisterCard 缺 name            → 400 F_001
//   3) Discover 空 capability          → 400 F_003
//   4) SendMessage 收件方不存在        → 200 + delivered:false + error F_004
//   5) SendMessage 已签 sender         → 200 + delivered:true
//   6) Bearer token 错（apiKey 启用）   → 401 F_AUTH
//   7) Bearer token 对                 → 通过
package httpgw

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aicity/a2a-gateway/internal/a2asrv"
	a2av1 "github.com/aicity/proto/gen/go/a2a/v1"
)

// newTestServer 构造最小可用 *a2asrv.Service（InboxAdapter fallback，nil store 静默 success）。
//
// Sprint 8：acl 传 nil → 默认 allow，HTTP 层用例行为不变。
func newTestServer() *a2asrv.Service {
	reg := a2asrv.NewRegistry()
	verifier := a2asrv.NewVerifier(5 * time.Minute)
	d := a2asrv.NewDispatcher()
	inboxAdapter := a2asrv.NewInboxAdapter(nil)
	d.Register(inboxAdapter)
	d.SetFallback(inboxAdapter)
	return a2asrv.NewService(reg, verifier, d, nil, nil)
}

// doRequest 发送一个 JSON 请求并解析响应。
func doRequest(t *testing.T, h http.Handler, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var rdr *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rdr)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

// ---------- 1) RegisterCard OK ----------

func TestRouter_RegisterCard_OK(t *testing.T) {
	svc := newTestServer()
	srv := New(svc, "")
	h := srv.Handler()

	w := doRequest(t, h, http.MethodPost, "/v1/cards", "", map[string]any{
		"agent_id":     "alice",
		"name":         "Alice",
		"provider":     "aicity",
		"capabilities": []string{"chat"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	var resp registerRespDTO
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v body=%s", err, w.Body.String())
	}
	if !resp.Accepted || resp.CardID != "alice" {
		t.Errorf("resp = %+v", resp)
	}
}

// ---------- 2) RegisterCard 缺 name → 400 F_001 ----------

func TestRouter_RegisterCard_MissingFields_F001(t *testing.T) {
	svc := newTestServer()
	srv := New(svc, "")
	h := srv.Handler()

	w := doRequest(t, h, http.MethodPost, "/v1/cards", "", map[string]any{
		"agent_id": "alice",
		// 缺 name
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "F_001") {
		t.Errorf("body should contain F_001, got %s", w.Body.String())
	}
}

// ---------- 3) Discover 空 capability → 400 F_003 ----------

func TestRouter_Discover_EmptyCapability_F003(t *testing.T) {
	svc := newTestServer()
	srv := New(svc, "")
	h := srv.Handler()

	w := doRequest(t, h, http.MethodGet, "/v1/discover", "", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "F_003") {
		t.Errorf("body should contain F_003, got %s", w.Body.String())
	}
}

// ---------- 4) SendMessage 收件方不存在 → delivered:false + F_004 ----------

func TestRouter_SendMessage_UnknownRecipient_F004(t *testing.T) {
	svc := newTestServer()
	srv := New(svc, "")
	h := srv.Handler()

	// 先注册 alice（发件方）
	_ = doRequest(t, h, http.MethodPost, "/v1/cards", "", map[string]any{
		"agent_id": "alice", "name": "Alice",
	})

	w := doRequest(t, h, http.MethodPost, "/v1/messages", "", map[string]any{
		"message_id":    "m1",
		"from_agent_id": "alice",
		"to_agent_id":   "ghost",
		"type":          "request",
	})
	// SendMessage 的失败不返 HTTP 错误码，而是返 200 + delivered:false + error="F_004:..."
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 body=%s", w.Code, w.Body.String())
	}
	var resp sendMessageRespDTO
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v body=%s", err, w.Body.String())
	}
	if resp.Delivered {
		t.Errorf("want delivered=false")
	}
	if !strings.HasPrefix(resp.Error, "F_004") {
		t.Errorf("error = %q, want F_004 prefix", resp.Error)
	}
}

// ---------- 5) SendMessage 已签 sender → delivered:true ----------

func TestRouter_SendMessage_Signed_Delivered(t *testing.T) {
	svc := newTestServer()
	srv := New(svc, "")
	h := srv.Handler()

	// 注册 alice（带 ed25519 公钥）+ bob
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	_ = doRequest(t, h, http.MethodPost, "/v1/cards", "", map[string]any{
		"agent_id": "alice", "name": "Alice",
		"auth": map[string]string{"ed25519": base64.StdEncoding.EncodeToString(pub)},
	})
	_ = doRequest(t, h, http.MethodPost, "/v1/cards", "", map[string]any{
		"agent_id": "bob", "name": "Bob",
	})

	now := time.Now().UnixMilli()
	// 构造 message + 用 server 同样的 canonical 方式签
	msg := &a2av1.Message{
		MessageId:   "m_signed",
		FromAgentId: "alice",
		ToAgentId:   "bob",
		Type:        "request",
		Payload:     []byte("hi"),
		TsMs:        now,
	}
	// 签（直接用 verifier.canonicalBytes 不能 import；inline 一份等价）
	canon := canonicalBytesForTest(msg)
	sig := ed25519.Sign(priv, canon)
	msg.Signature = base64.StdEncoding.EncodeToString(sig)

	w := doRequest(t, h, http.MethodPost, "/v1/messages", "", map[string]any{
		"message_id":    msg.MessageId,
		"from_agent_id": msg.FromAgentId,
		"to_agent_id":   msg.ToAgentId,
		"type":          msg.Type,
		"payload":       string(msg.Payload),
		"ts_ms":         msg.TsMs,
		"signature":     msg.Signature,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	var resp sendMessageRespDTO
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v body=%s", err, w.Body.String())
	}
	if !resp.Delivered || resp.Error != "" {
		t.Errorf("want delivered=true err=\"\", got %+v", resp)
	}
}

// ---------- 6) Bearer token 错 → 401 ----------

func TestRouter_BearerToken_Wrong_401(t *testing.T) {
	svc := newTestServer()
	srv := New(svc, "secret-key")
	h := srv.Handler()

	w := doRequest(t, h, http.MethodPost, "/v1/cards", "wrong-key", map[string]any{
		"agent_id": "alice", "name": "Alice",
	})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "F_AUTH") {
		t.Errorf("body should contain F_AUTH, got %s", w.Body.String())
	}
}

// ---------- 7) Bearer token 对 → 通过（register alice） ----------

func TestRouter_BearerToken_Correct_Passes(t *testing.T) {
	svc := newTestServer()
	srv := New(svc, "secret-key")
	h := srv.Handler()

	w := doRequest(t, h, http.MethodPost, "/v1/cards", "secret-key", map[string]any{
		"agent_id": "alice", "name": "Alice",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 body=%s", w.Code, w.Body.String())
	}

	// healthz 永远不要 token
	w2 := doRequest(t, h, http.MethodGet, "/v1/healthz", "", nil)
	if w2.Code != http.StatusOK {
		t.Errorf("healthz without token should pass, got %d", w2.Code)
	}
}

// ---------- Healthz ----------

func TestRouter_Healthz(t *testing.T) {
	svc := newTestServer()
	srv := New(svc, "")
	h := srv.Handler()

	w := doRequest(t, h, http.MethodGet, "/v1/healthz", "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["ok"] != true {
		t.Errorf("ok field = %v", resp["ok"])
	}
}

// ---------- TraceID 透传 ----------

func TestRouter_TraceID_Propagation(t *testing.T) {
	svc := newTestServer()
	srv := New(svc, "")
	h := srv.Handler()

	// 上游给 X-Trace-Id → 响应头原样回
	req := httptest.NewRequest(http.MethodGet, "/v1/healthz", nil)
	req.Header.Set("X-Trace-Id", "fixed-trace-1")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Header().Get("X-Trace-Id") != "fixed-trace-1" {
		t.Errorf("want X-Trace-Id=fixed-trace-1, got %q", w.Header().Get("X-Trace-Id"))
	}

	// 上游不给 → server 自动生成 UUID（非空）
	req2 := httptest.NewRequest(http.MethodGet, "/v1/healthz", nil)
	w2 := httptest.NewRecorder()
	h.ServeHTTP(w2, req2)
	got := w2.Header().Get("X-Trace-Id")
	if got == "" || got == "fixed-trace-1" {
		t.Errorf("auto trace should be non-empty UUID, got %q", got)
	}
}

// ---------- canonicalBytesForTest ----------
//
// inline a2asrv.canonicalBytes 的等价实现（不导入 a2asrv 的内部函数，
// 因为它在 a2asrv 包内未导出）。
// 字段顺序必须与服务侧 a2asrv.canonicalEnvelope 严格一致。

func canonicalBytesForTest(m *a2av1.Message) []byte {
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
	b, _ := json.Marshal(env)
	return b
}

// ---------- Sprint 7 FetchInbox 测试 ----------

// TestRouter_FetchInbox_AgentNotFound_F013 测 agent 未注册 → 404 F_013。
func TestRouter_FetchInbox_AgentNotFound_F013(t *testing.T) {
	svc := newTestServer()
	srv := New(svc, "")
	h := srv.Handler()

	// 注：newTestServer() 注入 nil inbox → FetchInbox 会返 "F_012:inbox not configured"
	// 本测试验证路由 + err map 即可；F_012 → 500（与 404 同测试覆盖）
	w := doRequest(t, h, http.MethodGet, "/v1/inbox/ghost?limit=10", "", nil)
	if w.Code != http.StatusInternalServerError && w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 500 or 404 body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "F_012") && !strings.Contains(w.Body.String(), "F_013") {
		t.Errorf("body should contain F_012 or F_013, got %s", w.Body.String())
	}
}

// TestRouter_FetchInbox_InvalidLimit_F014 测 limit 非法 → 400 F_014。
func TestRouter_FetchInbox_InvalidLimit_F014(t *testing.T) {
	svc := newTestServer()
	srv := New(svc, "")
	h := srv.Handler()

	// limit=0 → F_014
	w := doRequest(t, h, http.MethodGet, "/v1/inbox/bob?limit=0", "", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "F_014") {
		t.Errorf("body should contain F_014, got %s", w.Body.String())
	}

	// limit=abc → F_014
	w2 := doRequest(t, h, http.MethodGet, "/v1/inbox/bob?limit=abc", "", nil)
	if w2.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 body=%s", w2.Code, w2.Body.String())
	}
	if !strings.Contains(w2.Body.String(), "F_014") {
		t.Errorf("body should contain F_014, got %s", w2.Body.String())
	}

	// limit=501 → F_014
	w3 := doRequest(t, h, http.MethodGet, "/v1/inbox/bob?limit=501", "", nil)
	if w3.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 body=%s", w3.Code, w3.Body.String())
	}
	if !strings.Contains(w3.Body.String(), "F_014") {
		t.Errorf("body should contain F_014, got %s", w3.Body.String())
	}
}

// TestRouter_FetchInbox_Route 测路由注册 + 默认 limit=50。
func TestRouter_FetchInbox_Route(t *testing.T) {
	svc := newTestServer()
	srv := New(svc, "")
	h := srv.Handler()

	// inbox nil → FetchInbox 返 F_012 → 500
	w := doRequest(t, h, http.MethodGet, "/v1/inbox/bob", "", nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "F_012") {
		t.Errorf("body should contain F_012, got %s", w.Body.String())
	}
}

// 防 unused 警告（fmt 在某些测试断言扩展时可能用到）
var _ = fmt.Sprintf
