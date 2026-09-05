package httpgw

import (
	"fmt"
	"net/http"

	a2av1 "github.com/aicity/proto/gen/go/a2a/v1"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/status"
)

// Server 类型定义在 router.go；这里放 trace_idKey + handler 方法。

// traceIDKey 是 gin.Context 上 trace_id 的 key（与 router.go 同 const）。
const traceIDKey = "trace_id"

// traceIDFromCtx 从 gin.Context 读 trace_id；空返 ""。
func traceIDFromCtx(c *gin.Context) string {
	return c.GetString(traceIDKey)
}

// RegisterCard POST /v1/cards。
//   - 200 + {"accepted":true,"card_id":"..."} 成功
//   - 400 + F_001 缺字段
//   - 400 + F_006 auth.ed25519 解析失败
//   - 401 Bearer token 错（如果 apiKey 启用）
func (s *Server) RegisterCard(c *gin.Context) {
	var req cardDTO
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorEnvelope("F_001", "bad_request:"+err.Error(), traceIDFromCtx(c)))
		return
	}
	resp, err := s.svc.RegisterCard(c.Request.Context(), req.toProto())
	if err != nil {
		// gRPC status → 统一 envelope
		st, _ := status.FromError(err)
		c.JSON(fCodeToHTTP(st.Message()), errorEnvelope(extractFCode(st.Message()), extractDetail(st.Message()), traceIDFromCtx(c)))
		return
	}
	c.JSON(http.StatusOK, registerRespDTO{
		Accepted: resp.GetAccepted(),
		CardID:   resp.GetCardId(),
	})
}

// Discover GET /v1/discover?capability=...&city_filter=...
//   - 200 + {"cards":[...],"trace_id":"..."}
//   - 400 + F_003 capability 空
func (s *Server) Discover(c *gin.Context) {
	capability := c.Query("capability")
	if capability == "" {
		c.JSON(http.StatusBadRequest, errorEnvelope("F_003", "capability required", traceIDFromCtx(c)))
		return
	}
	resp, err := s.svc.Discover(c.Request.Context(), &a2av1.DiscoverRequest{
		Capability: capability,
		CityFilter: c.Query("city_filter"),
	})
	if err != nil {
		st, _ := status.FromError(err)
		c.JSON(fCodeToHTTP(st.Message()), errorEnvelope(extractFCode(st.Message()), extractDetail(st.Message()), traceIDFromCtx(c)))
		return
	}
	cards := make([]cardDTO, 0, len(resp.GetCards()))
	for _, p := range resp.GetCards() {
		cards = append(cards, cardFromProto(p))
	}
	c.JSON(http.StatusOK, discoverRespDTO{Cards: cards, TraceID: traceIDFromCtx(c)})
}

// SendMessage POST /v1/messages。
//   - 200 + {"delivered":true,"reply":{...}} 成功
//   - 200 + {"delivered":false,"error":"F_004..."} 失败（仿 gRPC MessageResponse.Error）
//   - 401 Bearer token 错（如果 apiKey 启用）
func (s *Server) SendMessage(c *gin.Context) {
	var req messageDTO
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorEnvelope("F_001", "bad_request:"+err.Error(), traceIDFromCtx(c)))
		return
	}
	resp, err := s.svc.SendMessage(c.Request.Context(), req.toProto())
	if err != nil {
		st, _ := status.FromError(err)
		c.JSON(fCodeToHTTP(st.Message()), errorEnvelope(extractFCode(st.Message()), extractDetail(st.Message()), traceIDFromCtx(c)))
		return
	}
	c.JSON(http.StatusOK, sendMessageRespDTO{Delivered: resp.GetDelivered(), Error: resp.GetError()})
}

// Healthz GET /v1/healthz —— 永远 200 ok，不依赖下游（用于 LB / k8s liveness）。
func (s *Server) Healthz(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"ok": true, "service": "a2a-gateway-http"})
}

// FetchInbox GET /v1/inbox/:agent_id?limit=&cursor=&mark_read=（Sprint 7）。
//
// 查询参数：
//   - limit:    默认 50，最大 500；非法 → F_014
//   - cursor:   增量分页（上一批的 next_cursor）
//   - mark_read: true = 拉取即标已读
//
// 错误码 → HTTP 状态映射见 errmap.go：
//   F_001 → 400（agent_id 空；理论上 :agent_id 已保证非空，本分支兜底）
//   F_013 → 404（agent 未注册）
//   F_014 → 400（limit 非法）
//   F_012 → 500（inbox 读失败）
func (s *Server) FetchInbox(c *gin.Context) {
	agentID := c.Param("agent_id")
	limitStr := c.Query("limit")
	cursor := c.Query("cursor")
	markRead := c.Query("mark_read") == "true"

	limit := int32(50)
	if limitStr != "" {
		var n int
		if _, err := fmt.Sscanf(limitStr, "%d", &n); err != nil {
			c.JSON(http.StatusBadRequest, errorEnvelope("F_014", "limit not integer: "+limitStr, traceIDFromCtx(c)))
			return
		}
		if n < 1 || n > 500 {
			c.JSON(http.StatusBadRequest, errorEnvelope("F_014", "limit out of range [1, 500]", traceIDFromCtx(c)))
			return
		}
		limit = int32(n)
	}

	resp, err := s.svc.FetchInbox(c.Request.Context(), &a2av1.FetchInboxRequest{
		AgentId:  agentID,
		Limit:    limit,
		Cursor:   cursor,
		MarkRead: markRead,
	})
	if err != nil {
		st, _ := status.FromError(err)
		c.JSON(fCodeToHTTP(st.Message()), errorEnvelope(extractFCode(st.Message()), extractDetail(st.Message()), traceIDFromCtx(c)))
		return
	}

	out := fetchInboxRespDTO{
		Messages:   make([]messageDTO, 0, len(resp.GetMessages())),
		NextCursor: resp.GetNextCursor(),
		TraceID:    traceIDFromCtx(c),
	}
	for _, m := range resp.GetMessages() {
		out.Messages = append(out.Messages, messageFromProto(m))
	}
	c.JSON(http.StatusOK, out)
}
