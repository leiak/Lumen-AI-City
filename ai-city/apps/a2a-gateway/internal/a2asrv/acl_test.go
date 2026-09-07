// ACL 单元测试（Sprint 8）—— 无 PG 依赖，用 mock denyChecker。
//
// 覆盖不变量：
//   1. nil ACL / nil store → 放行（向后兼容 Sprint 7 的既有调用路径）
//   2. deny 表无命中 → 放行（默认 allow）
//   3. deny 命中 → F_016
//   4. 后端查询出错 → F_016（fail-closed，不静默放行）
package a2asrv

import (
	"context"
	"errors"
	"strings"
	"testing"

	a2av1 "github.com/aicity/proto/gen/go/a2a/v1"
)

// mockDenyChecker 是 denyChecker 的可编程替身，并记录最后一次调用参数。
type mockDenyChecker struct {
	denied bool
	err    error

	gotAgentID  string
	gotPeerCity string
	calls       int
}

func (m *mockDenyChecker) IsDenied(ctx context.Context, agentID, peerCity string) (bool, error) {
	m.calls++
	m.gotAgentID = agentID
	m.gotPeerCity = peerCity
	return m.denied, m.err
}

func aliceCard() *a2av1.AgentCard {
	return &a2av1.AgentCard{AgentId: "alice", Name: "Alice", CityId: "beijing"}
}

func bobCard() *a2av1.AgentCard {
	return &a2av1.AgentCard{AgentId: "bob", Name: "Bob", CityId: "shanghai"}
}

// ---------- 1) nil-safe：放行 ----------

func TestACL_NilStore_PassThrough(t *testing.T) {
	cases := []struct {
		name string
		acl  *ACL
	}{
		{"nil ACL pointer", nil},
		{"zero-value ACL", &ACL{}},
		{"NewACL(nil)", NewACL(nil)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.acl.CheckDeliver(context.Background(), aliceCard(), bobCard()); err != nil {
				t.Errorf("CheckDeliver = %v, want nil (pass-through)", err)
			}
		})
	}
}

// NewACL(nil) 必须真正短路，而不是把 nil *ACLStore 包成非 nil 接口。
// 若回归，CheckDeliver 会 panic 而非返回 nil。
func TestACL_NewACL_TypedNil_NoPanic(t *testing.T) {
	var nilStore *ACLStore
	acl := NewACL(nilStore)
	if acl.store != nil {
		t.Fatal("NewACL(typed nil) stored a non-nil interface holding a nil pointer")
	}
	if err := acl.CheckDeliver(context.Background(), aliceCard(), bobCard()); err != nil {
		t.Errorf("CheckDeliver = %v, want nil", err)
	}
}

// sender / recipient 为 nil 时也不得 panic。
func TestACL_NilCards_PassThrough(t *testing.T) {
	m := &mockDenyChecker{denied: true} // 即便后端会拒，nil card 也不该走到后端
	acl := &ACL{store: m}

	if err := acl.CheckDeliver(context.Background(), nil, bobCard()); err != nil {
		t.Errorf("nil sender: got %v, want nil", err)
	}
	if err := acl.CheckDeliver(context.Background(), aliceCard(), nil); err != nil {
		t.Errorf("nil recipient: got %v, want nil", err)
	}
	if m.calls != 0 {
		t.Errorf("store called %d times for nil cards, want 0", m.calls)
	}
}

// ---------- 2) 默认 allow ----------

func TestACL_DefaultAllow_NoPolicy(t *testing.T) {
	m := &mockDenyChecker{denied: false}
	acl := &ACL{store: m}

	if err := acl.CheckDeliver(context.Background(), aliceCard(), bobCard()); err != nil {
		t.Errorf("CheckDeliver = %v, want nil (default allow)", err)
	}
	if m.calls != 1 {
		t.Errorf("store calls = %d, want 1", m.calls)
	}
	// 查询键：发件方 agent_id × 收件方 city_id
	if m.gotAgentID != "alice" {
		t.Errorf("agentID = %q, want alice (sender)", m.gotAgentID)
	}
	if m.gotPeerCity != "shanghai" {
		t.Errorf("peerCity = %q, want shanghai (recipient city)", m.gotPeerCity)
	}
}

// ---------- 3) deny 命中 → F_016 ----------

func TestACL_Deny_Match(t *testing.T) {
	m := &mockDenyChecker{denied: true}
	acl := &ACL{store: m}

	err := acl.CheckDeliver(context.Background(), aliceCard(), bobCard())
	if err == nil {
		t.Fatal("CheckDeliver = nil, want F_016 denial")
	}
	if !strings.HasPrefix(err.Error(), "F_016:") {
		t.Errorf("error = %q, want F_016: prefix", err.Error())
	}
}

// ---------- 4) 后端出错 → fail-closed F_016 ----------

func TestACL_StoreError_FailsClosed(t *testing.T) {
	m := &mockDenyChecker{err: errors.New("pg down")}
	acl := &ACL{store: m}

	err := acl.CheckDeliver(context.Background(), aliceCard(), bobCard())
	if err == nil {
		t.Fatal("CheckDeliver = nil on store error, want F_016 (fail-closed)")
	}
	if !strings.HasPrefix(err.Error(), "F_016:") {
		t.Errorf("error = %q, want F_016: prefix", err.Error())
	}
	// 原因应可追溯（%w 包裹）
	if !strings.Contains(err.Error(), "pg down") {
		t.Errorf("error = %q, want underlying cause preserved", err.Error())
	}
}

// 收件方 city_id 为空（旧 card）时，查询用空串 —— 由 ACLStore 的
// `peer_city = $2 OR peer_city = ''` 决定是否命中全局 deny。
func TestACL_EmptyRecipientCity_QueriesEmptyString(t *testing.T) {
	m := &mockDenyChecker{denied: false}
	acl := &ACL{store: m}

	legacy := &a2av1.AgentCard{AgentId: "carol", Name: "Carol"} // 无 city_id
	if err := acl.CheckDeliver(context.Background(), aliceCard(), legacy); err != nil {
		t.Errorf("CheckDeliver = %v, want nil", err)
	}
	if m.gotPeerCity != "" {
		t.Errorf("peerCity = %q, want empty string", m.gotPeerCity)
	}
}
