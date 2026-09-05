// HTTPAdapter 转发单测：用 httptest.NewServer 假 openclaw / workbuddy agent。
//
// 5 用例：
//   1) 200 + JSON reply → 返 reply（含 swap 检查）
//   2) 204 No Content    → 返 nil, nil（fire-and-forget 接受）
//   3) 500 Internal     → F_010 + reason 含 "500"
//   4) server 已关闭    → F_010 + reason 含 network err
//   5) 慢 server（>5s） → F_010 + context deadline exceeded
package a2asrv

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	a2av1 "github.com/aicity/proto/gen/go/a2a/v1"
)

// startFakeInbox 起一个 httptest server 模拟 /inbox 端点。
//   - handler: 自定义响应（status + body + delay）
//   - gotBody: 收到 POST body 后写入此 chan（用于断言）
func startFakeInbox(t *testing.T, handler http.HandlerFunc, gotBody chan<- string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/inbox", func(w http.ResponseWriter, r *http.Request) {
		if gotBody != nil {
			b, _ := io.ReadAll(r.Body)
			gotBody <- string(b)
		}
		handler(w, r)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// ---------- 1) 200 + JSON reply → 返 reply ----------

func TestHTTPAdapter_Deliver_OK_WithReply(t *testing.T) {
	bodyCh := make(chan string, 1)
	srv := startFakeInbox(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"message_id": "reply-1",
			"from_agent_id": "bob",
			"to_agent_id": "alice",
			"type": "event",
			"payload": "hi back",
			"ts_ms": 1700000000000
		}`))
	}, bodyCh)

	a := NewHTTPAdapter("openclaw", "openclaw", NewHTTPClient(2*time.Second))
	rec := &a2av1.AgentCard{AgentId: "bob", Provider: "openclaw", Url: srv.URL}
	msg := &a2av1.Message{
		MessageId:   "m1",
		FromAgentId: "alice",
		ToAgentId:   "bob",
		Type:        "request",
		Payload:     []byte("hello"),
		TsMs:        1700000000000,
	}

	reply, err := a.Deliver(context.Background(), rec, msg)
	if err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if reply == nil {
		t.Fatal("reply nil")
	}
	if reply.GetMessageId() != "reply-1" {
		t.Errorf("reply.id = %q", reply.GetMessageId())
	}
	if string(reply.GetPayload()) != "hi back" {
		t.Errorf("reply.payload = %q", reply.GetPayload())
	}
	if reply.GetFromAgentId() != "bob" || reply.GetToAgentId() != "alice" {
		t.Errorf("reply swap wrong: from=%q to=%q", reply.GetFromAgentId(), reply.GetToAgentId())
	}

	// 验证 server 收到的 body 是 messageDTO 形式（payload 为字符串）
	gotBody := <-bodyCh
	var sent map[string]any
	if err := json.Unmarshal([]byte(gotBody), &sent); err != nil {
		t.Fatalf("decode sent body: %v body=%s", err, gotBody)
	}
	if sent["message_id"] != "m1" || sent["from_agent_id"] != "alice" || sent["to_agent_id"] != "bob" {
		t.Errorf("sent body fields wrong: %+v", sent)
	}
	if sent["payload"] != "hello" {
		t.Errorf("payload = %v, want \"hello\"", sent["payload"])
	}
}

// ---------- 2) 204 No Content → 返 nil, nil ----------

func TestHTTPAdapter_Deliver_204_ReturnsNilReply(t *testing.T) {
	srv := startFakeInbox(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}, nil)

	a := NewHTTPAdapter("openclaw", "openclaw", NewHTTPClient(2*time.Second))
	rec := &a2av1.AgentCard{AgentId: "bob", Provider: "openclaw", Url: srv.URL}
	msg := &a2av1.Message{MessageId: "m1", FromAgentId: "alice", ToAgentId: "bob"}

	reply, err := a.Deliver(context.Background(), rec, msg)
	if err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if reply != nil {
		t.Errorf("204 should return nil reply, got %+v", reply)
	}
}

// ---------- 3) 500 Internal → F_010 ----------

func TestHTTPAdapter_Deliver_500_F010(t *testing.T) {
	srv := startFakeInbox(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("oops"))
	}, nil)

	a := NewHTTPAdapter("workbuddy", "workbuddy", NewHTTPClient(2*time.Second))
	rec := &a2av1.AgentCard{AgentId: "bob", Provider: "workbuddy", Url: srv.URL}
	msg := &a2av1.Message{MessageId: "m1", FromAgentId: "alice", ToAgentId: "bob"}

	_, err := a.Deliver(context.Background(), rec, msg)
	if err == nil {
		t.Fatal("want error")
	}
	ae, ok := err.(*AdapterError)
	if !ok {
		t.Fatalf("want *AdapterError, got %T: %v", err, err)
	}
	if ae.Code != "F_010" {
		t.Errorf("code = %q, want F_010", ae.Code)
	}
	if !strings.Contains(ae.Reason, "500") {
		t.Errorf("reason should contain \"500\", got %q", ae.Reason)
	}
}

// ---------- 4) server 已关闭 → F_010（network err） ----------

func TestHTTPAdapter_Deliver_ClosedServer_F010(t *testing.T) {
	srv := startFakeInbox(t, func(w http.ResponseWriter, r *http.Request) {}, nil)
	url := srv.URL
	srv.Close() // 立即关

	a := NewHTTPAdapter("openclaw", "openclaw", NewHTTPClient(2*time.Second))
	rec := &a2av1.AgentCard{AgentId: "bob", Provider: "openclaw", Url: url}
	msg := &a2av1.Message{MessageId: "m1", FromAgentId: "alice", ToAgentId: "bob"}

	_, err := a.Deliver(context.Background(), rec, msg)
	if err == nil {
		t.Fatal("want error")
	}
	ae, ok := err.(*AdapterError)
	if !ok {
		t.Fatalf("want *AdapterError, got %T: %v", err, err)
	}
	if ae.Code != "F_010" {
		t.Errorf("code = %q, want F_010", ae.Code)
	}
	// reason 应含 "upstream" + 网络错误描述
	if !strings.Contains(ae.Reason, "upstream") {
		t.Errorf("reason should contain \"upstream\", got %q", ae.Reason)
	}
}

// ---------- 5) 慢 server → 5s timeout → F_010 ----------

func TestHTTPAdapter_Deliver_Timeout_F010(t *testing.T) {
	if testing.Short() {
		t.Skip("skip timeout test in -short mode")
	}
	var reqCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&reqCount, 1)
		// 模拟慢 server：sleep 3s 后响应（client 500ms timeout 应先触发）
		select {
		case <-time.After(3 * time.Second):
			w.WriteHeader(http.StatusOK)
		case <-r.Context().Done():
			// client 断开后立即返回，避免 srv.Close 等
		}
	}))
	t.Cleanup(srv.Close)

	a := NewHTTPAdapter("openclaw", "openclaw", NewHTTPClient(500*time.Millisecond))
	rec := &a2av1.AgentCard{AgentId: "bob", Provider: "openclaw", Url: srv.URL}
	msg := &a2av1.Message{MessageId: "m1", FromAgentId: "alice", ToAgentId: "bob"}

	start := time.Now()
	_, err := a.Deliver(context.Background(), rec, msg)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("want timeout error")
	}
	ae, ok := err.(*AdapterError)
	if !ok {
		t.Fatalf("want *AdapterError, got %T: %v", err, err)
	}
	if ae.Code != "F_010" {
		t.Errorf("code = %q, want F_010", ae.Code)
	}
	// 应在 ~500ms 左右超时（不要真等 3s）
	if elapsed > 2*time.Second {
		t.Errorf("elapsed = %v, want ~500ms (timeout too long)", elapsed)
	}
}

// ---------- Supports ----------

func TestHTTPAdapter_Supports(t *testing.T) {
	a := NewHTTPAdapter("openclaw", "openclaw", nil)
	if !a.Supports("openclaw") {
		t.Error("openclaw should support openclaw")
	}
	if a.Supports("workbuddy") {
		t.Error("openclaw should NOT support workbuddy")
	}
	if a.Supports("") {
		t.Error("openclaw should NOT support empty provider")
	}
}

// ---------- URL empty → F_010 ----------

func TestHTTPAdapter_Deliver_EmptyURL_F010(t *testing.T) {
	a := NewHTTPAdapter("openclaw", "openclaw", nil)
	rec := &a2av1.AgentCard{AgentId: "bob", Provider: "openclaw"} // 无 URL
	msg := &a2av1.Message{MessageId: "m1", FromAgentId: "alice", ToAgentId: "bob"}

	_, err := a.Deliver(context.Background(), rec, msg)
	if err == nil {
		t.Fatal("want error")
	}
	ae, ok := err.(*AdapterError)
	if !ok || ae.Code != "F_010" {
		t.Errorf("want F_010 AdapterError, got %T: %v", err, err)
	}
	if !strings.Contains(ae.Reason, "URL") {
		t.Errorf("reason should mention URL, got %q", ae.Reason)
	}
}
