// EchoAdapter（Sprint 5.5 + Sprint 6）：
//   - EchoAdapter：aicity 内部 agent 的兜底；Sprint 5 MVP 的 echo 行为保留
//   - OpenClawStub / WorkbuddyStub 在 Sprint 6 搬到 adapter_http.go，由真 HTTPAdapter 替代
//
// EchoAdapter 行为与 Sprint 5 MVP 的 Stream echo 一致：
//   swap from/to + type→event + 清空 signature；不持久化、不做 inbox。
//   Sprint 6+ 接 PG inbox 时改写本文件即可，Dispatcher / Service 零改动。
package a2asrv

import (
	"context"
	"log"

	a2av1 "github.com/aicity/proto/gen/go/a2a/v1"
)

// EchoAdapter 是 aicity provider 的兜底 adapter（也兼任 fallback）。
type EchoAdapter struct{}

func (EchoAdapter) Supports(provider string) bool {
	// 兜底：空 provider + aicity 都接；其它 provider 不抢。
	return provider == "" || provider == "aicity"
}

func (EchoAdapter) Deliver(ctx context.Context, recipient *a2av1.AgentCard, msg *a2av1.Message) (*a2av1.Message, error) {
	log.Printf("[a2asrv] EchoAdapter %s → %s (msg=%s, %d bytes)",
		msg.GetFromAgentId(), msg.GetToAgentId(), msg.GetMessageId(), len(msg.GetPayload()))
	return &a2av1.Message{
		MessageId:      msg.GetMessageId(),
		FromAgentId:    msg.GetToAgentId(),
		ToAgentId:      msg.GetFromAgentId(),
		ConversationId: msg.GetConversationId(),
		Type:           "event",
		Payload:        msg.GetPayload(),
		TsMs:           msg.GetTsMs(),
		TraceId:        msg.GetTraceId(),
		Signature:      "", // 清空避免误用
	}, nil
}
