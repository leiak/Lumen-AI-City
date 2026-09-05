// 默认 adapter 实现（Sprint 5.5）：
//   - EchoAdapter：aicity 内部 agent 的兜底；Sprint 5 MVP 的 echo 行为搬过来
//   - OpenClawStub / WorkbuddyStub：占位 adapter，Sprint 6 替换为真 HTTP 转发
//
// 三个 adapter 都把消息 swap from/to + type="event" 后回传（仅 EchoAdapter
// 是真行为；stub 只 log + 返 nil 表示占位成功）。
package a2asrv

import (
	"context"
	"log"

	a2av1 "github.com/aicity/proto/gen/go/a2a/v1"
)

// EchoAdapter 是 aicity provider 的兜底 adapter（也兼任 fallback）。
//
// 行为与 Sprint 5 MVP 的 Stream echo 一致：
//   swap from/to + type→event + 清空 signature；不持久化、不做 inbox。
//   Sprint 6+ 接 PG inbox 时改写本文件即可，Dispatcher / Service 零改动。
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

// OpenClawStub 是 openclaw provider 的占位 adapter。
// Sprint 6 替换为真 HTTP 转发（POST 到 recipient.URL）。
type OpenClawStub struct{}

func (OpenClawStub) Supports(provider string) bool { return provider == "openclaw" }

func (OpenClawStub) Deliver(ctx context.Context, recipient *a2av1.AgentCard, msg *a2av1.Message) (*a2av1.Message, error) {
	log.Printf("[a2asrv] OpenClawStub (placeholder) msg=%s to=%s url=%s",
		msg.GetMessageId(), msg.GetToAgentId(), recipient.GetUrl())
	// 占位实现：返 nil 表示 fire-and-forget 已"接受"。
	// Sprint 6 替换为真 HTTP POST + 等待 ack。
	return nil, nil
}

// WorkbuddyStub 同 OpenClawStub；拆开便于 Sprint 6 单独路由策略。
type WorkbuddyStub struct{}

func (WorkbuddyStub) Supports(provider string) bool { return provider == "workbuddy" }

func (WorkbuddyStub) Deliver(ctx context.Context, recipient *a2av1.AgentCard, msg *a2av1.Message) (*a2av1.Message, error) {
	log.Printf("[a2asrv] WorkbuddyStub (placeholder) msg=%s to=%s url=%s",
		msg.GetMessageId(), msg.GetToAgentId(), recipient.GetUrl())
	return nil, nil
}
