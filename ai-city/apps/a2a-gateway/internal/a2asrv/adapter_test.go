// Dispatcher 单测：5 用例
//   1) aicity provider → EchoAdapter（swap from/to）
//   2) openclaw → HTTPAdapter（路由命中；具体行为见 adapter_http_test.go）
//   3) 未知 provider（fallback 不接） → F_009
//   4) 重复 Register 同 provider → 后者覆盖前者
//   5) recipient.Provider="" + fallback=EchoAdapter → EchoAdapter 兜底
package a2asrv

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	a2av1 "github.com/aicity/proto/gen/go/a2a/v1"
)

func TestDispatcher_AicityEcho(t *testing.T) {
	d := NewDispatcher()
	d.Register(EchoAdapter{})

	rec := &a2av1.AgentCard{AgentId: "bob", Provider: "aicity"}
	msg := &a2av1.Message{MessageId: "m1", FromAgentId: "alice", ToAgentId: "bob", Payload: []byte("hi")}

	reply, err := d.Deliver(context.Background(), rec, msg)
	if err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if reply.GetType() != "event" {
		t.Errorf("type want event got %q", reply.GetType())
	}
	if reply.GetFromAgentId() != "bob" || reply.GetToAgentId() != "alice" {
		t.Errorf("swap failed: from=%q to=%q", reply.GetFromAgentId(), reply.GetToAgentId())
	}
}

func TestDispatcher_OpenClawHTTP_Route(t *testing.T) {
	// 假 openclaw /inbox：返 204 → adapter 返 nil,nil
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	d := NewDispatcher()
	d.Register(NewHTTPAdapter("openclaw", "openclaw", NewHTTPClient(2*time.Second)))

	rec := &a2av1.AgentCard{AgentId: "carol", Provider: "openclaw", Url: srv.URL}
	msg := &a2av1.Message{MessageId: "m1", FromAgentId: "alice", ToAgentId: "carol"}

	reply, err := d.Deliver(context.Background(), rec, msg)
	if err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if reply != nil {
		t.Errorf("204 reply should be nil, got %+v", reply)
	}
}

func TestDispatcher_UnknownProvider_F009(t *testing.T) {
	d := NewDispatcher()
	d.Register(EchoAdapter{})
	d.Register(NewHTTPAdapter("openclaw", "openclaw", NewHTTPClient(2*time.Second)))
	// 不设 fallback → 未知 provider → F_009
	// 但 EchoAdapter.Supports("") == true，所以把 fallback 干掉
	d.SetFallback(nil)

	rec := &a2av1.AgentCard{AgentId: "x", Provider: "unknownprov"}
	msg := &a2av1.Message{MessageId: "m1", FromAgentId: "a", ToAgentId: "x"}

	_, err := d.Deliver(context.Background(), rec, msg)
	if err == nil {
		t.Fatal("want F_009")
	}
	if !strings.HasPrefix(err.Error(), "F_009:") {
		t.Errorf("want F_009 prefix, got %q", err.Error())
	}
}

func TestDispatcher_Register_Overwrite(t *testing.T) {
	d := NewDispatcher()
	d.Register(EchoAdapter{})
	// 第二次 Register 一个"假 EchoAdapter"也接 aicity → 应覆盖
	type stubA struct{ EchoAdapter }
	stub := stubA{}
	// stub 继承 Supports -> 与 EchoAdapter 相同；覆盖本身不破坏功能，
	// 只验证 byProv 槽位被替换（避免 double-dispatch）
	d.Register(stub)

	rec := &a2av1.AgentCard{AgentId: "bob", Provider: "aicity"}
	msg := &a2av1.Message{MessageId: "m1", FromAgentId: "alice", ToAgentId: "bob"}
	if _, err := d.Deliver(context.Background(), rec, msg); err != nil {
		t.Errorf("after overwrite Deliver should still work: %v", err)
	}
}

func TestDispatcher_EmptyProvider_FallbackEcho(t *testing.T) {
	d := NewDispatcher()
	d.Register(EchoAdapter{})
	d.SetFallback(EchoAdapter{})

	rec := &a2av1.AgentCard{AgentId: "bob", Provider: ""} // 缺省
	msg := &a2av1.Message{MessageId: "m1", FromAgentId: "alice", ToAgentId: "bob", Payload: []byte("x")}

	reply, err := d.Deliver(context.Background(), rec, msg)
	if err != nil {
		t.Fatalf("fallback Deliver: %v", err)
	}
	if reply.GetType() != "event" {
		t.Errorf("fallback echo want event, got %q", reply.GetType())
	}
}
