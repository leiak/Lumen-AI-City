// ACL 授权门（Sprint 8）。
//
// 语义：**默认 allow**。仅当 a2a_acl_policy 里存在
// (sender.agent_id, recipient.city_id, 'deny') 行时才阻断投递。
// 这保证 Sprint 5-7 的既有行为（无策略 = 全通）不被打破。
//
// 集成点：service.go 的 SendMessage / Stream —— 在 verifier.Verify 之后、
// dispatcher.Deliver 之前。签名先验证再授权：先确认"你是谁"，再判断"你能不能"。
//
// 错误码：
//   F_015 Discover 被 ACL 拒绝（本 sprint 不在 wire 上触发；
//         Discover 尚无 caller 身份概念，留给 Sprint 9+ caller-identity）
//   F_016 投递被 ACL 拒绝（SendMessage / Stream）
//
// fail-closed：ACL 后端查询失败时返回 error（→ F_016 拒投），
// 而非静默放行。宁可拒投可重试，不可漏放。
package a2asrv

import (
	"context"
	"errors"
	"fmt"

	a2av1 "github.com/aicity/proto/gen/go/a2a/v1"
)

// denyChecker 抽象 deny list 后端，便于单测注入 mock。
// 生产实现是 *ACLStore（aclstore_pg.go）。
type denyChecker interface {
	IsDenied(ctx context.Context, agentID, peerCity string) (bool, error)
}

// ACL 是投递授权门。store 为 nil 时所有检查放行（向后兼容 / 测试）。
type ACL struct {
	store denyChecker
}

// NewACL 构造 ACL。
//
// store 为 nil（未配置 PG）→ 返回一个恒放行的 ACL，等价于 Sprint 7 行为。
// 注意显式 nil 检查：不能直接塞进接口字段，否则 nil *ACLStore 会被包成
// 非 nil 接口值，调用时 panic。
func NewACL(store *ACLStore) *ACL {
	if store == nil {
		return &ACL{}
	}
	return &ACL{store: store}
}

// CheckDeliver 判断 sender 是否被允许向 recipient 所在城邦投递。
//
// 返回 nil = 放行；非 nil = 拒绝，error 串以 "F_016:" 开头供调用方直接映射。
//
// nil-safe：ACL 为 nil、store 为 nil、sender/recipient 为 nil 时一律放行
// —— 让未注入 ACL 的既有测试与调用路径零改动。
func (a *ACL) CheckDeliver(ctx context.Context, sender, recipient *a2av1.AgentCard) error {
	if a == nil || a.store == nil {
		return nil
	}
	if sender == nil || recipient == nil {
		return nil
	}
	denied, err := a.store.IsDenied(ctx, sender.GetAgentId(), recipient.GetCityId())
	if err != nil {
		// fail-closed：查不出来就不放行
		return fmt.Errorf("F_016:acl check failed: %w", err)
	}
	if denied {
		return errors.New("F_016:delivery denied by ACL")
	}
	return nil
}
