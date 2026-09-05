// Package a2asrv 实现 A2A 联邦协议的内存注册表与 gRPC service。
//
// Registry 是 MVP 阶段的 AgentCard 注册表：
//   - sync.RWMutex + map（重启即清，无持久化）
//   - Register 幂等：同 agent_id 重复注册覆盖（errCode=F_002）
//   - Discover 按 capability 过滤（MVP 忽略 cityFilter）
//
// 错误码体系遵循 docs/06-A2A协议.md §20.10：
//   F_001 agent_id/name 缺失
//   F_002 agent_id 已存在（仍 accepted=true，幂等覆盖）
//   F_003 capability 为空
//   F_004 收件方未注册（service 层用）
//   F_005 发件方未注册（service 层用）
//   F_006 auth["ed25519"] 非空但公钥解析失败（Sprint 5.5）
package a2asrv

import (
	"encoding/base64"
	"log"
	"sync"

	a2av1 "github.com/aicity/proto/gen/go/a2a/v1"
	"crypto/ed25519"
)

// Registry 内存 AgentCard 注册表。线程安全。
type Registry struct {
	mu    sync.RWMutex
	cards map[string]*a2av1.AgentCard
}

// NewRegistry 构造空注册表。
func NewRegistry() *Registry {
	return &Registry{cards: make(map[string]*a2av1.AgentCard)}
}

// Register 注册 / 覆盖 AgentCard。
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
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.cards[card.GetAgentId()]; exists {
		r.cards[card.GetAgentId()] = card
		return true, "F_002"
	}
	r.cards[card.GetAgentId()] = card
	return true, ""
}

// validateEd25519 检查 auth["ed25519"] 若非空必须能解为 32 字节公钥。
// 不在 Register 里验签 —— Register 只校验格式；签名验证在 service.SendMessage。
func validateEd25519(card *a2av1.AgentCard) string {
	auth := card.GetAuth()
	if len(auth) == 0 {
		return ""
	}
	b64, ok := auth["ed25519"]
	if !ok || b64 == "" {
		return ""
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || len(raw) != ed25519.PublicKeySize {
		return "F_006"
	}
	return ""
}

// Discover 按 capability 过滤返回 AgentCard 列表。
// cityFilter 在 MVP 忽略（多城邦路由留给 Sprint 6+）。
// 返回 (cards, errCode)：capability 为空时 errCode=F_003。
func (r *Registry) Discover(capability, cityFilter string) ([]*a2av1.AgentCard, string) {
	if capability == "" {
		return nil, "F_003"
	}
	if cityFilter != "" {
		// MVP 不实现，留 warn 便于后续排障
		log.Printf("[a2asrv] Discover cityFilter=%q 暂忽略（MVP 仅内存）", cityFilter)
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*a2av1.AgentCard, 0)
	for _, c := range r.cards {
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
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.cards[agentID]
	return c, ok
}

// Size 返回当前注册数（测试 / metrics 用）。
func (r *Registry) Size() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.cards)
}
