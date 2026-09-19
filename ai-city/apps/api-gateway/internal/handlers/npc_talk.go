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
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/aicity/api-gateway/internal/npc"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

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
	var req npcTalkReq
	if err := c.ShouldBindJSON(&req); err != nil {
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
