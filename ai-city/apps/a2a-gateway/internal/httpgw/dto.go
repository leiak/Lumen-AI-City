package httpgw

import (
	a2av1 "github.com/aicity/proto/gen/go/a2a/v1"
)

// JSON DTO（mirror struct）。
//
// 设计原因：proto 序列化 `payload bytes` 走 protojson 时会序列化为
// base64 padding 形式（"aGk="），与 canonical 签名用的 RawStdEncoding
// 不一致。故 HTTP gateway 不走 protojson，直接用 mirror struct + snake_case。
//
// 字段顺序对齐 packages/sdk-go/client.go 现有契约（向后兼容）。
// 后续 Sprint 7+ 若引入独立 OpenAPI schema，可单独维护，本文件保持最小集。

// cardDTO 对应 AgentCard（POST /v1/cards 请求体）。
type cardDTO struct {
	AgentID      string            `json:"agent_id"`
	Name         string            `json:"name"`
	Description  string            `json:"description,omitempty"`
	URL          string            `json:"url,omitempty"`
	Provider     string            `json:"provider,omitempty"`
	Version      string            `json:"version,omitempty"`
	Capabilities []string          `json:"capabilities,omitempty"`
	Auth         map[string]string `json:"auth,omitempty"`
}

func (c *cardDTO) toProto() *a2av1.AgentCard {
	if c == nil {
		return nil
	}
	return &a2av1.AgentCard{
		AgentId:      c.AgentID,
		Name:         c.Name,
		Description:  c.Description,
		Url:          c.URL,
		Provider:     c.Provider,
		Version:      c.Version,
		Capabilities: c.Capabilities,
		Auth:         c.Auth,
	}
}

// cardFromProto 把 *a2av1.AgentCard 翻成 cardDTO（Discover 响应）。
func cardFromProto(p *a2av1.AgentCard) cardDTO {
	if p == nil {
		return cardDTO{}
	}
	return cardDTO{
		AgentID:      p.GetAgentId(),
		Name:         p.GetName(),
		Description:  p.GetDescription(),
		URL:          p.GetUrl(),
		Provider:     p.GetProvider(),
		Version:      p.GetVersion(),
		Capabilities: p.GetCapabilities(),
		Auth:         p.GetAuth(),
	}
}

// discoverRespDTO 是 GET /v1/discover 响应（前端数组形式 + trace_id）。
type discoverRespDTO struct {
	Cards   []cardDTO `json:"cards"`
	TraceID string    `json:"trace_id,omitempty"`
}

// messageDTO 对应 a2av1.Message。
//   - Payload: 字符串（base64.RawStdEncoding 形态），与 canonical 签名一致
//   - Signature: base64 标准编码字符串
type messageDTO struct {
	MessageID      string `json:"message_id"`
	FromAgentID    string `json:"from_agent_id"`
	ToAgentID      string `json:"to_agent_id"`
	ConversationID string `json:"conversation_id,omitempty"`
	Type           string `json:"type"`
	Payload        string `json:"payload,omitempty"`
	TsMs           int64  `json:"ts_ms,omitempty"`
	TraceID        string `json:"trace_id,omitempty"`
	Signature      string `json:"signature,omitempty"`
}

func (m *messageDTO) toProto() *a2av1.Message {
	if m == nil {
		return nil
	}
	return &a2av1.Message{
		MessageId:      m.MessageID,
		FromAgentId:    m.FromAgentID,
		ToAgentId:      m.ToAgentID,
		ConversationId: m.ConversationID,
		Type:           m.Type,
		Payload:        []byte(m.Payload),
		TsMs:           m.TsMs,
		TraceId:        m.TraceID,
		Signature:      m.Signature,
	}
}

func messageFromProto(p *a2av1.Message) messageDTO {
	if p == nil {
		return messageDTO{}
	}
	return messageDTO{
		MessageID:      p.GetMessageId(),
		FromAgentID:    p.GetFromAgentId(),
		ToAgentID:      p.GetToAgentId(),
		ConversationID: p.GetConversationId(),
		Type:           p.GetType(),
		Payload:        string(p.GetPayload()),
		TsMs:           p.GetTsMs(),
		TraceID:        p.GetTraceId(),
		Signature:      p.GetSignature(),
	}
}

// registerRespDTO 是 POST /v1/cards 响应。
type registerRespDTO struct {
	Accepted bool   `json:"accepted"`
	CardID   string `json:"card_id"`
}

// sendMessageRespDTO 是 POST /v1/messages 响应。
//   - 成功 → delivered:true
//   - 失败 → delivered:false + error envelope（不走本 DTO，走 errorEnvelope）
type sendMessageRespDTO struct {
	Delivered bool        `json:"delivered"`
	Error     string      `json:"error,omitempty"`
	Reply     *messageDTO `json:"reply,omitempty"`
}
