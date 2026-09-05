// Package a2asrv 实现 A2A 联邦协议的注册表与 gRPC service。
//
// Registry（Sprint 7 重构）：
//   - 生产环境：NewRegistryFromCardStore 包装 PG-backed CardStore（持久化）
//   - 测试环境：NewRegistry 返回带 in-memory fallback 的 Registry（无 PG 依赖）
//   - 公共 API（Register/Get/Discover/Size）保持不变 → 现有 68 单测无需改业务代码
//
// 错误码体系遵循 docs/06-A2A协议.md §20.10：
//   F_001 agent_id/name 缺失
//   F_002 agent_id 已存在（仍 accepted=true，幂等覆盖）
//   F_003 capability 为空
//   F_004 收件方未注册（service 层用）
//   F_005 发件方未注册（service 层用）
//   F_006 auth["ed25519"] 非空但公钥解析失败
package a2asrv

import (
	"context"
	"log"
	"sync"

	a2av1 "github.com/aicity/proto/gen/go/a2a/v1"
)

// Registry 是 AgentCard 注册表的对外入口。
//
// 内部实现：
//   - store != nil：委托给 PG-backed CardStore（生产路径）
//   - store == nil：fallback 到 in-memory map（测试路径，无 PG 依赖）
type Registry struct {
	store *CardStore // nil = in-memory mode

	mu    sync.RWMutex
	mem   map[string]*a2av1.AgentCard // only used when store == nil
}

// NewRegistry 构造 in-memory Registry（仅测试用）。
//
// 生产代码必须用 NewRegistryFromCardStore 注入 PG-backed store。
func NewRegistry() *Registry {
	return &Registry{mem: make(map[string]*a2av1.AgentCard)}
}

// NewRegistryFromCardStore 包装 PG-backed CardStore（生产路径）。
func NewRegistryFromCardStore(s *CardStore) *Registry {
	if s == nil {
		log.Printf("[a2asrv] NewRegistryFromCardStore: nil store → fall back to in-memory (should not happen in prod)")
		return NewRegistry()
	}
	return &Registry{store: s}
}

// Register 注册 / 覆盖 AgentCard。
//
// 返回 (accepted, errCode)：
//   - 缺失字段 → accepted=false, errCode=F_001
//   - auth["ed25519"] 非空但公钥解析失败 → accepted=false, errCode=F_006
//   - 重复 agent_id → accepted=true, errCode=F_002（幂等覆盖）
//   - 首次注册 → accepted=true, errCode=""
func (r *Registry) Register(card *a2av1.AgentCard) (bool, string) {
	if card == nil || card.GetAgentId() == "" || card.GetName() == "" {
		return false, "F_001"
	}
	if code := validateEd25519(card); code != "" {
		return false, code
	}
	if r.store != nil {
		// 生产路径：走 PG
		return r.store.Register(context.Background(), card)
	}
	// 测试路径：in-memory
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.mem[card.GetAgentId()]; exists {
		r.mem[card.GetAgentId()] = card
		return true, "F_002"
	}
	r.mem[card.GetAgentId()] = card
	return true, ""
}

// Discover 按 capability 过滤返回 AgentCard 列表。
// cityFilter 在 Sprint 7 仍忽略（warn log；ACL 留给 Sprint 7+）。
// 返回 (cards, errCode)：capability 为空时 errCode=F_003。
func (r *Registry) Discover(capability, cityFilter string) ([]*a2av1.AgentCard, string) {
	if capability == "" {
		return nil, "F_003"
	}
	if cityFilter != "" {
		log.Printf("[a2asrv] Registry.Discover cityFilter=%q 暂忽略（ACL 留给 Sprint 7+）", cityFilter)
	}
	if r.store != nil {
		return r.store.Discover(context.Background(), capability, cityFilter)
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*a2av1.AgentCard, 0)
	for _, c := range r.mem {
		for _, cap := range c.GetCapabilities() {
			if cap == capability {
				out = append(out, c)
				break
			}
		}
	}
	return out, ""
}

// Get 按 agent_id 查 card。返回 (card, ok)。
func (r *Registry) Get(agentID string) (*a2av1.AgentCard, bool) {
	if agentID == "" {
		return nil, false
	}
	if r.store != nil {
		return r.store.Get(context.Background(), agentID)
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.mem[agentID]
	return c, ok
}

// Size 返回当前注册数（测试 / metrics 用）。
func (r *Registry) Size() int {
	if r.store != nil {
		return r.store.Size(context.Background())
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.mem)
}

// CardStoreGetter 暴露内部 CardStore 访问（main.go / 测试用）。
// 返回 nil 表示 in-memory 模式。
func (r *Registry) CardStoreGetter() *CardStore {
	return r.store
}
