// Package handlers - npc_talk (Sprint 12)
//
// POST /v1/npc/talk — given {npc_id, player_id, choice_id}, look up the NPC's
// talk_tree (loaded once at startup from YAML) and return the matching node's
// reply envelope.
//
// Side effect: best-effort publish to Redis channel `aicity:npc_dialogue` so
// ws-gateway fans out to the player's browser. A publish failure is logged but
// does NOT fail the HTTP response — the response body itself is the source
// of truth for the in-process caller (web client / agent-core).
//
// Error code convention (mirrors a2a-gateway F_xxx):
//   - NPC_001: NPC not found (npc_id missing from registry)      → 404
//   - NPC_002: bad request (missing fields / unknown choice_id)  → 400
//   - NPC_003: internal error (reserved; publish failures are NOT NPC_003
//     because they are best-effort — see above)
package handlers

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aicity/api-gateway/internal/npc"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// mintSessionID returns a 16-byte (32 hex char) session id like
// "sess-7f3b2e1c9a4d8e2f". Uses crypto/rand for uniqueness.
//
// B1-T07 followup: per spec §3.5 line 250, session_id is optional; when
// omitted, api-gateway (A 城) mints one before forwarding to a2a-gateway.
// The minted id is also re-marshaled into rawBody so the upstream request
// carries the same value.
func mintSessionID() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return "sess-" + hex.EncodeToString(buf[:]), nil
}

// crossCityStreamBodyCap is the upstream body cap for the SSE relay (matches
// a2a-gateway's POST /v1/federation/say_stream cap; see
// apps/a2a-gateway/internal/httpgw/say_stream.go maxBodyBytes).
const crossCityStreamBodyCap = 64 * 1024

// crossCityStreamReadTimeout caps the upstream connect+read for the initial
// response; the SSE body itself is open-ended and tears down via ctx.
const crossCityStreamReadTimeout = 5 * time.Second

// RedisPublisher is the minimal interface npc_talk needs from the redis client.
// Defined here (not in the handlers package's shared interface file) because
// only this handler publishes fire-and-forget — keep the surface tight.
type RedisPublisher interface {
	Publish(ctx context.Context, channel, payload string) *redis.IntCmd
}

// NPCTalkHandler serves POST /v1/npc/talk.
type NPCTalkHandler struct {
	Trees      map[string]*npc.Tree
	Redis      RedisPublisher
	Logger     *zap.Logger
	NPCChannel string
}

// NewNPCTalkHandler wires the handler. channel is typically "aicity:npc_dialogue".
func NewNPCTalkHandler(trees map[string]*npc.Tree, rdb RedisPublisher, log *zap.Logger, channel string) *NPCTalkHandler {
	return &NPCTalkHandler{
		Trees:      trees,
		Redis:      rdb,
		Logger:     log,
		NPCChannel: channel,
	}
}

type npcTalkReq struct {
	NpcID    string `json:"npc_id"`
	PlayerID string `json:"player_id"`
	ChoiceID string `json:"choice_id"`
}

type dialogOptionDTO struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type npcTalkResp struct {
	NpcID           string            `json:"npc_id"`
	PlayerID        string            `json:"player_id"`
	TileID          string            `json:"tile_id"`
	Say             string            `json:"say"`
	Options         []dialogOptionDTO `json:"options"`
	ReplyToChoiceID string            `json:"reply_to_choice_id"`
}

// Handle is the gin.HandlerFunc for POST /v1/npc/talk.
func (h *NPCTalkHandler) Handle(c *gin.Context) {
	// B1-T07: read the full body once so cross-city forwarding (SSE) can
	// forward the raw bytes (incl. session_id, context, player_input that
	// the a2a-gateway SayStreamHandler requires). Local NPCs are unaffected —
	// they only need req.{NpcID,PlayerID,ChoiceID}.
	rawBody, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "NPC_002", "detail": err.Error()})
		return
	}

	var req npcTalkReq
	if err := json.Unmarshal(rawBody, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "NPC_002", "detail": err.Error()})
		return
	}
	if req.NpcID == "" || req.PlayerID == "" || req.ChoiceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":  "NPC_002",
			"detail": "npc_id, player_id, choice_id are required",
		})
		return
	}

	// B1-T07: 跨城 NPC（id 含 `_b_`）转发到 a2a-gateway /v1/federation/say_stream
	// （SSE 透传）。替代旧的 forwardCrossCity（POST /v1/cross_city/talk 整段）。
	// 降级路径：SSE 不可达 → 502 R_016 → web 端改走整段（a2a-gateway 仍保留）。
	if strings.Contains(req.NpcID, "_b_") {
		h.forwardCrossCityStream(c, req.NpcID, rawBody)
		return
	}

	tree, ok := h.Trees[req.NpcID]
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{
			"error":  "NPC_001",
			"detail": "NPC not found: " + req.NpcID,
		})
		return
	}
	node, ok := tree.Lookup(req.ChoiceID)
	if !ok {
		// Sprint13 增补：未知/过期 choice → 优雅回退到模板 `talk_tree.default_say`
		// （不打断对话，回一句 default 并照常发布到玩家浏览器）。只有连 default_say
		// 都没配的 NPC 才维持 400。
		if tree.DefaultSay == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":  "NPC_002",
				"detail": "unknown choice_id: " + req.ChoiceID,
			})
			return
		}
		node = npc.Node{Say: tree.DefaultSay}
	}

	opts := make([]dialogOptionDTO, len(node.Options))
	for i, o := range node.Options {
		opts[i] = dialogOptionDTO{ID: o.ID, Text: o.Text}
	}

	resp := npcTalkResp{
		NpcID:           req.NpcID,
		PlayerID:        req.PlayerID,
		TileID:          tree.HomeTile,
		Say:             node.Say,
		Options:         opts,
		ReplyToChoiceID: req.ChoiceID,
	}
	c.JSON(http.StatusOK, resp)

	// Best-effort publish — log on failure, but never surface as HTTP error.
	//
	// Publishes the INNER payload only (no outer envelope). ws-gateway wraps it
	// uniformly with type/trace_id/ts_ms before fanning out to the browser.
	// This matches the world-engine pattern for `aicity:player:moved`.
	innerPayload := map[string]any{
		"npc_id":             resp.NpcID,
		"player_id":          resp.PlayerID,
		"tile_id":            resp.TileID,
		"say":                resp.Say,
		"options":            opts,
		"reply_to_choice_id": resp.ReplyToChoiceID,
		"ts_ms":              time.Now().UnixMilli(),
		"trace_id":           c.GetHeader("X-Trace-ID"),
	}
	payload, _ := json.Marshal(innerPayload)
	if err := h.Redis.Publish(c.Request.Context(), h.NPCChannel, string(payload)).Err(); err != nil {
		h.Logger.Warn("npc publish failed",
			zap.String("npc_id", req.NpcID),
			zap.String("player_id", req.PlayerID),
			zap.String("choice_id", req.ChoiceID),
			zap.String("channel", h.NPCChannel),
			zap.Error(err),
		)
	}
}

// HandleByID serves POST /v1/npc/:id/talk — the spec-shaped endpoint.
//
// Per docs/superpowers/specs/2026-09-08-sprint12-min-slice-design.md §1.1 /
// §2.3 the canonical shape is "npc_id in URL path, body only carries
// {player_id, choice_id}". The earlier POST /v1/npc/talk route (with npc_id
// in the body) is kept as an alias for backward compatibility with callers
// that already speak that shape — see router.go for the two registrations.
//
// Behaviour mirrors Handle() exactly: same lookup + default_say fallback +
// best-effort Redis publish on aicity:npc_dialogue. Error code mapping is
// identical (NPC_001/002).
func (h *NPCTalkHandler) HandleByID(c *gin.Context) {
	npcID := c.Param("id")
	if npcID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":  "NPC_002",
			"detail": "npc id is required in URL path",
		})
		return
	}

	// B1-T07: 同样读 raw body 一次，让跨城 SSE 透传能把 session_id/context 转发到
	// a2a-gateway SayStreamHandler。
	rawBody, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "NPC_002", "detail": err.Error()})
		return
	}

	var req struct {
		PlayerID string `json:"player_id"`
		ChoiceID string `json:"choice_id"`
	}
	if err := json.Unmarshal(rawBody, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "NPC_002", "detail": err.Error()})
		return
	}

	// B1-T07: 跨城 NPC → forwardCrossCityStream（SSE 透传），替换 forwardCrossCity。
	if strings.Contains(npcID, "_b_") {
		h.forwardCrossCityStream(c, npcID, rawBody)
		return
	}

	if req.PlayerID == "" || req.ChoiceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":  "NPC_002",
			"detail": "player_id, choice_id are required",
		})
		return
	}

	tree, ok := h.Trees[npcID]
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{
			"error":  "NPC_001",
			"detail": "NPC not found: " + npcID,
		})
		return
	}
	node, ok := tree.Lookup(req.ChoiceID)
	if !ok {
		// 与 Handle() 同款：未知 choice → default_say 兜底（不打断对话）。
		if tree.DefaultSay == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":  "NPC_002",
				"detail": "unknown choice_id: " + req.ChoiceID,
			})
			return
		}
		node = npc.Node{Say: tree.DefaultSay}
	}

	opts := make([]dialogOptionDTO, len(node.Options))
	for i, o := range node.Options {
		opts[i] = dialogOptionDTO{ID: o.ID, Text: o.Text}
	}

	resp := npcTalkResp{
		NpcID:           npcID,
		PlayerID:        req.PlayerID,
		TileID:          tree.HomeTile,
		Say:             node.Say,
		Options:         opts,
		ReplyToChoiceID: req.ChoiceID,
	}
	c.JSON(http.StatusOK, resp)

	// Best-effort publish — 与 Handle() 同款语义（详见上面 Handle 注释）。
	innerPayload := map[string]any{
		"npc_id":             resp.NpcID,
		"player_id":          resp.PlayerID,
		"tile_id":            resp.TileID,
		"say":                resp.Say,
		"options":            opts,
		"reply_to_choice_id": resp.ReplyToChoiceID,
		"ts_ms":              time.Now().UnixMilli(),
		"trace_id":           c.GetHeader("X-Trace-ID"),
	}
	payload, _ := json.Marshal(innerPayload)
	if err := h.Redis.Publish(c.Request.Context(), h.NPCChannel, string(payload)).Err(); err != nil {
		h.Logger.Warn("npc publish failed (by-id)",
			zap.String("npc_id", npcID),
			zap.String("player_id", req.PlayerID),
			zap.String("choice_id", req.ChoiceID),
			zap.String("channel", h.NPCChannel),
			zap.Error(err),
		)
	}
}

// npcInfoResp is the GET /v1/npcs/:id response: the NPC's first-turn (root)
// say + options, so a client can seed a conversation without re-parsing the
// talk_tree YAML (single source of truth stays in packages/npc-templates).
type npcInfoResp struct {
	NpcID      string            `json:"npc_id"`
	Name       string            `json:"name"`
	HomeTileID string            `json:"home_tile_id"`
	Say        string            `json:"say"`
	Options    []dialogOptionDTO `json:"options"`
}

// HandleInfo serves GET /v1/npcs/:id.
// Returns 404 NPC_001 for an unknown npc_id.
// Known NPC without a root node: 200 with empty say/options (front-end falls back).
func (h *NPCTalkHandler) HandleInfo(c *gin.Context) {
	id := c.Param("id")
	tree, ok := h.Trees[id]
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "NPC_001", "detail": "NPC not found: " + id})
		return
	}
	say := tree.DefaultSay // Sprint13 增补：无 root 节点时回退默认开场，而非空
	opts := []dialogOptionDTO{}
	if n, found := tree.InitialNode(); found {
		say = n.Say
		opts = make([]dialogOptionDTO, len(n.Options))
		for i, o := range n.Options {
			opts[i] = dialogOptionDTO{ID: o.ID, Text: o.Text}
		}
	}
	c.JSON(http.StatusOK, npcInfoResp{
		NpcID:      tree.NpcID,
		Name:       tree.Name,
		HomeTileID: tree.HomeTile,
		Say:        say,
		Options:    opts,
	})
}

// forwardCrossCity 把跨城 NPC 对话请求转发到 a2a-gateway。
//
// 触发条件：npcID 含 "_b_"（跨城 NPC 命名约定：npc_<city>_<slug>）。
// 转发目标：A2A_HUB_URL env（默认 http://a2a-gateway:8083）+/v1/cross_city/talk。
//
// 错误码（与 a2a F_xxx 对齐）：
//   - 502 + F_010：a2a-gateway 不可达（网络错误）
//   - 透传 a2a-gateway 自身的 HTTP 状态码与响应体（包括 503+F_011 路由未命中）
//
// 不做本地 Redis publish：跨城 NPC 的 dialogue 由 a2a-gateway 端 publish 到
// 对应频道，ws-gateway 跨城 fanout 订阅 a2a.cross_city.event 再扇出给本城客户端。
//
// B1-T07 状态：仅保留方法以备作 fallback（plan 6 主路径已切到 forwardCrossCityStream
// SSE 透传）；当前 Handle/HandleByID 都不再调用它。如果 web 端 SSE 入口拒拿再考虑
// S 整段 fallback 路由。
func (h *NPCTalkHandler) forwardCrossCity(c *gin.Context, npcID string, body map[string]any) {
	a2aURL := os.Getenv("A2A_HUB_URL")
	if a2aURL == "" {
		a2aURL = "http://a2a-gateway:8083"
	}
	payload, err := json.Marshal(body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":  "NPC_003",
			"detail": "marshal cross-city payload: " + err.Error(),
		})
		return
	}
	resp, err := http.Post(a2aURL+"/v1/cross_city/talk", "application/json", bytes.NewReader(payload))
	if err != nil {
		h.Logger.Warn("cross-city forward failed",
			zap.String("npc_id", npcID),
			zap.String("a2a_url", a2aURL),
			zap.Error(err),
		)
		c.JSON(http.StatusBadGateway, gin.H{
			"code":    "F_010",
			"message": "a2a-gateway 不可达",
		})
		return
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		h.Logger.Warn("cross-city decode failed",
			zap.String("npc_id", npcID),
			zap.Int("status", resp.StatusCode),
			zap.Error(err),
		)
		c.JSON(http.StatusBadGateway, gin.H{
			"code":    "F_010",
			"message": "a2a-gateway 响应解析失败",
		})
		return
	}
	c.JSON(resp.StatusCode, result)
}

// forwardCrossCityStream 把跨城 NPC 对话请求以 SSE 形式透传到 a2a-gateway。
//
// B1-T07：替换 forwardCrossCity（Sint 整段）→ forwardCrossCityStream（SSE）。
// 调用方：Handle/HandleByID 当 npcID 含 `_b_` 时调用。处理流程：
//  1. npcID 必填校验（缺 → 400 R_009，不发上游请求）
//  2. body 必填校验：JSON 可解；缺 session_id 由本层 mint 一个（spec §3.5
//     line 250：session_id 可选；缺省由 A 城生成），保证 a2a-gateway
//     SayStreamHandler 入站 sid 永远非空
//  3. POST http://<A2A_HUB_URL>/v1/federation/say_stream，body 透传，附 Bearer
//     （env A2A_HTTP_API_KEY 非空时）
//  4. a2a-gateway 非 200 → 按原状态码 + body 透传给 web 客户端（不二次包 SSE）
//  5. 200 → 透传响应（text/event-stream 帧原样写入 client.Writer
//
// 错误码（与 a2a F_xxx 对齐）：
//   - 400 + R_009：npcID 为空 / body 不可读 / JSON 非法
//   - 500 + R_011：mint / re-marshal session_id 失败（极少见，crypto/rand 出错）
//   - 502 + R_016：a2a-gateway 不可达（网络错误 / 拨号超时）
//   - 透传 a2a-gateway 自身的 HTTP 状态码与 body（含 400/401/502 + 错误 JSON）
//
// 不做本地 Redis publish：跨城 NPC 的节拍包由 a2a-gateway publish 到 A 城 Redis
// `aicity:npc:say_stream`，ws-gateway 多频道订阅统一扇出。
func (h *NPCTalkHandler) forwardCrossCityStream(c *gin.Context, npcID string, rawBody []byte) {
	if npcID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "R_009",
			"message": "npc_id required",
		})
		return
	}

	a2aURL := os.Getenv("A2A_HUB_URL")
	if a2aURL == "" {
		a2aURL = "http://a2a-gateway:8083"
	}

	// Preflight: body 必须是合法 JSON，且 session_id 必须存在（a2a-gateway
	// SayStreamHandler 同款契约）。这里提前校验，避免白跑一次 HTTP 调用。
	if int64(len(rawBody)) > crossCityStreamBodyCap {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "R_009",
			"message": "body too large",
		})
		return
	}
	var peek map[string]any
	if err := json.Unmarshal(rawBody, &peek); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "R_009",
			"message": "invalid json: " + err.Error(),
		})
		return
	}
	if sid, _ := peek["session_id"].(string); sid == "" {
		// B1-T07 followup: spec §3.5 line 250 — session_id 可选；缺省由 A 城 mint。
		// a2a-gateway SayStreamHandler 仍要求非空；由 api-gateway 保证入站 sid 永远非空。
		newSID, err := mintSessionID()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    "R_011",
				"message": "sid mint fail: " + err.Error(),
			})
			return
		}
		peek["session_id"] = newSID
		// Re-marshal so the forwarded body includes the minted sid.
		rawBody, err = json.Marshal(peek)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    "R_011",
				"message": "sid re-marshal fail: " + err.Error(),
			})
			return
		}
	}

	// 构造上游 POST：context 用 client ctx，body 透传，附 Bearer。
	upstreamReq, err := http.NewRequestWithContext(c.Request.Context(),
		http.MethodPost,
		a2aURL+"/v1/federation/say_stream",
		bytes.NewReader(rawBody))
	if err != nil {
		h.Logger.Warn("cross-city stream build request failed",
			zap.String("npc_id", npcID),
			zap.String("a2a_url", a2aURL),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "R_016",
			"message": "build upstream request fail: " + err.Error(),
		})
		return
	}
	upstreamReq.Header.Set("Content-Type", "application/json")
	if apiKey := os.Getenv("A2A_HTTP_API_KEY"); apiKey != "" {
		upstreamReq.Header.Set("Authorization", "Bearer "+apiKey)
	}

	// SSE 是长连接：连接 / 读首响应用有限超时（crossCityStreamReadTimeout），
	// body 后续读取无超时，由 ctx 取消（web 断连 → a2a-gctx 收到 signal → 收尾）。
	client := &http.Client{Timeout: crossCityStreamReadTimeout}
	resp, err := client.Do(upstreamReq)
	if err != nil {
		h.Logger.Warn("cross-city stream forward failed",
			zap.String("npc_id", npcID),
			zap.String("a2a_url", a2aURL),
			zap.Error(err),
		)
		c.JSON(http.StatusBadGateway, gin.H{
			"code":    "R_016",
			"message": "a2a-gateway 不可达",
		})
		return
	}
	defer resp.Body.Close()

	// a2a-gateway 非 200：按原状态码 + body 透传（前端作为 JSON 错误处理）。
	if resp.StatusCode != http.StatusOK {
		c.Writer.Header().Set("Content-Type", "application/json")
		c.Writer.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(c.Writer, resp.Body)
		return
	}

	// SSE 透传：把 a2a-gateway 响应的 text/event-stream 原样写入 client。
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(c.Writer, resp.Body)
}
