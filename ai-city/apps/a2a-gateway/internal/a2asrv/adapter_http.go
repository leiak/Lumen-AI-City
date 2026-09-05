// Adapter 真 HTTP 转发（Sprint 6）。
//
// 替换 Sprint 5.5 的 OpenClawStub / WorkbuddyStub：
//   - HTTPAdapter { name, provider, client(*HTTPClient) }
//   - Deliver: POST {recipient.URL}/inbox + JSON body → 等 reply
//   - 失败 → *AdapterError{Code:"F_010", Reason:...}
//
// 设计要点：
//   - 同步阻塞 + 5s timeout（共享 *http.Client）
//   - 204 / 空 body → 返 (nil, nil) 表示 fire-and-forget 已接受
//   - 200 + JSON body → 解 messageDTO → 翻 proto Message 回传
//   - 4xx / 5xx → F_010 + 状态码
//   - decode 失败 → F_010 + 原因
//
// Sprint 6 不接 retry / circuit breaker；Sprint 7+ 视需要加。
package a2asrv

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	a2av1 "github.com/aicity/proto/gen/go/a2a/v1"
)

// HTTPClient 包装 *http.Client（共享连接池）。
//
// Transport 配置：
//   - MaxIdleConnsPerHost=4（联邦外 agent 通常少；按需调）
//   - 复用默认 Proxy / Dialer
type HTTPClient struct {
	*http.Client
}

// NewHTTPClient 构造 HTTPClient。
//   - timeout <= 0 → 5s 默认
func NewHTTPClient(timeout time.Duration) *HTTPClient {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	tr := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          32,
		MaxIdleConnsPerHost:   4,
		IdleConnTimeout:       60 * time.Second,
		TLSHandshakeTimeout:   3 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return &HTTPClient{Client: &http.Client{Transport: tr, Timeout: timeout}}
}

// HTTPAdapter 把外部联邦 agent（openclaw / workbuddy）的 /inbox 端点接到 A2A 协议。
//
// 选路规则（与 Dispatcher 接口一致）：
//   - Supports(provider) 仅当 provider == a.provider 时返回 true
//   - Deliver: POST {recipient.URL}/inbox
type HTTPAdapter struct {
	name     string        // log 用（"openclaw" / "workbuddy"）
	provider string        // aicity 默认 "" → 不被本 adapter 接管
	client   *HTTPClient   // 共享 client（连接池复用）
}

// NewHTTPAdapter 构造 HTTPAdapter。
//   - provider 空 → 仅作 fallback；不建议（用 EchoAdapter 兜底更便宜）
func NewHTTPAdapter(name, provider string, c *HTTPClient) *HTTPAdapter {
	if c == nil {
		c = NewHTTPClient(5 * time.Second)
	}
	return &HTTPAdapter{name: name, provider: provider, client: c}
}

// Supports 声明 provider 名（与 Sprint 5.5 providerNames 对齐）。
func (a *HTTPAdapter) Supports(provider string) bool {
	return provider == a.provider
}

// Deliver 把 msg POST 到 {recipient.URL}/inbox，等 reply。
//
// 返回：
//   - (nil, nil)           204 / 空 body → fire-and-forget 接受
//   - (*Message, nil)      200 + JSON reply
//   - (nil, *AdapterError) 任何失败 → F_010 + reason
func (a *HTTPAdapter) Deliver(ctx context.Context, recipient *a2av1.AgentCard, msg *a2av1.Message) (*a2av1.Message, error) {
	if recipient == nil || recipient.GetUrl() == "" {
		return nil, &AdapterError{Code: "F_010", Reason: "recipient URL empty"}
	}
	url := recipient.GetUrl() + "/inbox"

	// body: 用 messageToDTO（不走 protojson，避免 payload bytes → base64 padding 不一致）
	body, err := json.Marshal(messageToDTO(msg))
	if err != nil {
		return nil, &AdapterError{Code: "F_010", Reason: "marshal: " + err.Error()}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, &AdapterError{Code: "F_010", Reason: "build request: " + err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-A2A-Provider", a.provider)

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, &AdapterError{Code: "F_010", Reason: "upstream " + err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, &AdapterError{Code: "F_010", Reason: "upstream status " + strconv.Itoa(resp.StatusCode)}
	}

	// 204 或 ContentLength=0 → fire-and-forget
	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}
	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &AdapterError{Code: "F_010", Reason: "read body: " + err.Error()}
	}
	if len(rawBody) == 0 {
		return nil, nil
	}

	// 200 + JSON reply → 解 messageDTO
	var reply messageReplyDTO
	if err := json.Unmarshal(rawBody, &reply); err != nil {
		return nil, &AdapterError{Code: "F_010", Reason: "reply decode: " + err.Error()}
	}
	return reply.toProto(), nil
}

// ---------- DTO（与 httpgw.messageDTO 镜像，但只用于 outbound 序列化） ----------

// messageToDTO 把 a2av1.Message 翻成 JSON 形态（payload 是字符串，与 canonical 兼容）。
func messageToDTO(m *a2av1.Message) messageReplyDTO {
	if m == nil {
		return messageReplyDTO{}
	}
	return messageReplyDTO{
		MessageID:      m.GetMessageId(),
		FromAgentID:    m.GetFromAgentId(),
		ToAgentID:      m.GetToAgentId(),
		ConversationID: m.GetConversationId(),
		Type:           m.GetType(),
		Payload:        string(m.GetPayload()),
		TsMs:           m.GetTsMs(),
		TraceID:        m.GetTraceId(),
		Signature:      m.GetSignature(),
	}
}

// messageReplyDTO 是 /inbox POST 响应体（与发件方 message 一致字段）。
type messageReplyDTO struct {
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

func (r *messageReplyDTO) toProto() *a2av1.Message {
	if r == nil {
		return nil
	}
	return &a2av1.Message{
		MessageId:      r.MessageID,
		FromAgentId:    r.FromAgentID,
		ToAgentId:      r.ToAgentID,
		ConversationId: r.ConversationID,
		Type:           r.Type,
		Payload:        []byte(r.Payload),
		TsMs:           r.TsMs,
		TraceId:        r.TraceID,
		Signature:      r.Signature,
	}
}
