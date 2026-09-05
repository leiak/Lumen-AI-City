// InboxAdapter 单测（不依赖 PG）。
//
// 2 用例：
//   1) Supports("", "aicity") → true；其它 false
//   2) Deliver with nil store → 静默返 (nil, nil)；不 panic
package a2asrv

import (
	"context"
	"testing"

	a2av1 "github.com/aicity/proto/gen/go/a2a/v1"
)

func TestInboxAdapter_Supports(t *testing.T) {
	a := NewInboxAdapter(nil)
	if !a.Supports("") {
		t.Error("InboxAdapter should support empty provider")
	}
	if !a.Supports("aicity") {
		t.Error("InboxAdapter should support aicity")
	}
	if a.Supports("openclaw") {
		t.Error("InboxAdapter should NOT support openclaw")
	}
	if a.Supports("workbuddy") {
		t.Error("InboxAdapter should NOT support workbuddy")
	}
}

func TestInboxAdapter_Deliver_NilStore_SilentSuccess(t *testing.T) {
	a := NewInboxAdapter(nil)
	rec := &a2av1.AgentCard{AgentId: "bob", Provider: "aicity"}
	msg := &a2av1.Message{
		MessageId: "m1", FromAgentId: "alice", ToAgentId: "bob",
		Type: "request", Payload: []byte("hi"),
	}

	reply, err := a.Deliver(context.Background(), rec, msg)
	if err != nil {
		t.Fatalf("Deliver with nil store should be silent success, got err: %v", err)
	}
	if reply != nil {
		t.Errorf("Deliver should return nil reply (fire-and-forget), got %+v", reply)
	}
}

func TestInboxAdapter_Deliver_NilMessage_F011(t *testing.T) {
	a := NewInboxAdapter(nil)
	rec := &a2av1.AgentCard{AgentId: "bob", Provider: "aicity"}

	_, err := a.Deliver(context.Background(), rec, nil)
	if err == nil {
		t.Fatal("nil message should return error")
	}
	ae, ok := err.(*AdapterError)
	if !ok || ae.Code != "F_011" {
		t.Errorf("want F_011, got %T: %v", err, err)
	}
}
