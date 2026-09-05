// InboxAdapter（Sprint 7）：
//   - 替换 Sprint 5/6 的 EchoAdapter
//   - aicity 内部 agent 的兜底：写 a2a_inbox 后返 (nil, nil)，与 Sprint 6 HTTPAdapter 204 语义一致
//   - Dispatcher / Service 零改动（adapter.go 已注明 "return (nil,nil) for fire-and-forget"）
//
// 设计：
//   - Supports(provider) 仅当 provider == "" 或 "aicity" 时 true
//   - Deliver：append InboxStore；PG 失败 → *AdapterError{Code:"F_011"}（让 service 返 F_011）
//   - InboxAdapter 也作为 Dispatcher 的 fallback（未知 provider / 空 provider 的兜底）
package a2asrv

import (
	"context"
	"log"

	a2av1 "github.com/aicity/proto/gen/go/a2a/v1"
)

// InboxAdapter 是 aicity provider 的兜底 adapter：写 inbox 后返 (nil, nil)。
type InboxAdapter struct {
	store *InboxStore
}

// NewInboxAdapter 构造 InboxAdapter。
func NewInboxAdapter(store *InboxStore) *InboxAdapter {
	return &InboxAdapter{store: store}
}

// Supports 声明 provider 名：仅空 / aicity。
func (a *InboxAdapter) Supports(provider string) bool {
	return provider == "" || provider == "aicity"
}

// Deliver 把 msg 写入 a2a_inbox，返 (nil, nil) 表示 fire-and-forget 已 accepted。
//
//   - 收件方在线 / 离线统一走 inbox：在线 agent 后续通过 FetchInbox RPC / GET /v1/inbox/:id 拉取
//   - PG 写入失败 → *AdapterError{Code:"F_011", Reason: err.Error()}
//   - store == nil（测试场景）→ 静默返 (nil, nil)，跳过实际写入
func (a *InboxAdapter) Deliver(ctx context.Context, recipient *a2av1.AgentCard, msg *a2av1.Message) (*a2av1.Message, error) {
	if msg == nil {
		return nil, &AdapterError{Code: "F_011", Reason: "nil message"}
	}
	if a.store == nil {
		// 测试场景：nil store → 静默返 success（避免 test stub panic）
		log.Printf("[a2asrv] InboxAdapter %s → %s (msg=%s) — store nil, skip write",
			msg.GetFromAgentId(), msg.GetToAgentId(), msg.GetMessageId())
		return nil, nil
	}
	entry := inboxEntryFromMessage(msg, "")
	if err := a.store.Append(ctx, entry, ""); err != nil {
		return nil, &AdapterError{Code: "F_011", Reason: err.Error()}
	}
	log.Printf("[a2asrv] InboxAdapter %s → %s (msg=%s, %d bytes queued)",
		msg.GetFromAgentId(), msg.GetToAgentId(), msg.GetMessageId(), len(msg.GetPayload()))
	return nil, nil
}
