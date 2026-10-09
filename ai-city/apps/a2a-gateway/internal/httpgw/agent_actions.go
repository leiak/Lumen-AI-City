package httpgw

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
)

// RuntimeConfig points the HTTP gateway at local world services and the
// shared NPC templates. Empty WorldURL disables movement actions.
type RuntimeConfig struct {
	WorldURL       string
	NPCTemplateDir string
	CORSOrigins    []string
}

type agentMoveRequest struct {
	AgentID    string  `json:"agent_id"`
	TileID     string  `json:"tile_id"`
	FromTileID string  `json:"from_tile_id"`
	X          float32 `json:"x"`
	Y          float32 `json:"y"`
}

type npcOption struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type npcNode struct {
	Say     string      `json:"say"`
	Options []npcOption `json:"options"`
}

type npcBehaviorTree struct {
	NPCID      string             `json:"npc_id"`
	Name       string             `json:"name"`
	Initial    string             `json:"initial"`
	DefaultSay string             `json:"default_say"`
	Nodes      map[string]npcNode `json:"nodes"`
}

type rawNpcTemplate struct {
	NPCID      string `yaml:"npc_id"`
	Name       string `yaml:"name"`
	HomeTileID string `yaml:"home_tile_id"`
	TalkTree   struct {
		Initial    string                `yaml:"initial"`
		DefaultSay string                `yaml:"default_say"`
		Nodes      map[string]npcNodeDTO `yaml:"nodes"`
	} `yaml:"talk_tree"`
}

type npcNodeDTO struct {
	Say     string `yaml:"say"`
	Options []struct {
		ID   string `yaml:"id"`
		Text string `yaml:"text"`
	} `yaml:"options"`
}

type npcTalkRequest struct {
	AgentID string `json:"agent_id"`
	NPCID   string `json:"npc_id"`
	NodeID  string `json:"node_id"`
}

type npcTalkResponse struct {
	NPCID   string      `json:"npc_id"`
	NodeID  string      `json:"node_id"`
	Say     string      `json:"say"`
	Options []npcOption `json:"options"`
}

const npcActionTimeout = 5 * time.Second

// AgentMove proxies an authenticated A2A identity into the world movement API.
// Movement stays outside the human WebSocket protocol: agents use this HTTP
// action and receive the same world-engine broadcast as human clients.
func (s *Server) AgentMove(c *gin.Context) {
	var req agentMoveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorEnvelope("F_001", "bad_request:"+err.Error(), traceIDFromCtx(c)))
		return
	}
	if req.AgentID == "" {
		c.JSON(http.StatusBadRequest, errorEnvelope("F_001", "agent_id required", traceIDFromCtx(c)))
		return
	}
	if !s.svc.HasAgent(req.AgentID) {
		c.JSON(http.StatusForbidden, errorEnvelope("F_005", "agent not registered", traceIDFromCtx(c)))
		return
	}
	if req.TileID == "" {
		c.JSON(http.StatusBadRequest, errorEnvelope("F_001", "tile_id required", traceIDFromCtx(c)))
		return
	}
	if s.worldURL == "" {
		c.JSON(http.StatusServiceUnavailable, errorEnvelope("R_016", "world engine not configured", traceIDFromCtx(c)))
		return
	}

	body, _ := json.Marshal(map[string]any{
		"player_id":    req.AgentID,
		"from_tile_id": req.FromTileID,
		"to_tile_id":   req.TileID,
		"x":            req.X,
		"y":            req.Y,
	})
	ctx, cancel := context.WithTimeout(c.Request.Context(), npcActionTimeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, s.worldURL+"/v1/world/move", bytes.NewReader(body))
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorEnvelope("R_011", err.Error(), traceIDFromCtx(c)))
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		c.JSON(http.StatusBadGateway, errorEnvelope("R_016", "world engine unavailable: "+err.Error(), traceIDFromCtx(c)))
		return
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		c.JSON(http.StatusBadGateway, errorEnvelope("R_016", err.Error(), traceIDFromCtx(c)))
		return
	}
	c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), payload)
}

// NPCBehavior exposes the full shared talk_tree to A2A agents. This is read
// access for planning dialogue; the gateway still chooses which node is run.
func (s *Server) NPCBehavior(c *gin.Context) {
	npcID := c.Query("npc_id")
	if npcID == "" {
		c.JSON(http.StatusBadRequest, errorEnvelope("F_001", "npc_id required", traceIDFromCtx(c)))
		return
	}
	tree, err := loadNPCTree(s.npcTemplateDir, npcID)
	if err != nil {
		c.JSON(http.StatusNotFound, errorEnvelope("NPC_001", err.Error(), traceIDFromCtx(c)))
		return
	}
	c.JSON(http.StatusOK, tree)
}

// NPCTalk executes one deterministic behavior-tree node on behalf of a
// registered A2A agent. Agents cannot mutate the tree; they only request a node.
func (s *Server) NPCTalk(c *gin.Context) {
	var req npcTalkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorEnvelope("F_001", "bad_request:"+err.Error(), traceIDFromCtx(c)))
		return
	}
	if req.AgentID == "" || req.NPCID == "" {
		c.JSON(http.StatusBadRequest, errorEnvelope("F_001", "agent_id and npc_id required", traceIDFromCtx(c)))
		return
	}
	if !s.svc.HasAgent(req.AgentID) {
		c.JSON(http.StatusForbidden, errorEnvelope("F_005", "agent not registered", traceIDFromCtx(c)))
		return
	}
	tree, err := loadNPCTree(s.npcTemplateDir, req.NPCID)
	if err != nil {
		c.JSON(http.StatusNotFound, errorEnvelope("NPC_001", err.Error(), traceIDFromCtx(c)))
		return
	}
	nodeID := req.NodeID
	if nodeID == "" {
		nodeID = tree.Initial
	}
	node, ok := tree.Nodes[nodeID]
	if !ok {
		say := tree.DefaultSay
		if say == "" {
			c.JSON(http.StatusBadRequest, errorEnvelope("NPC_002", "unknown node_id: "+nodeID, traceIDFromCtx(c)))
			return
		}
		node = npcNode{Say: say}
	}
	c.JSON(http.StatusOK, npcTalkResponse{NPCID: req.NPCID, NodeID: nodeID, Say: node.Say, Options: node.Options})
}

// loadNPCTree reads one shared template. A gateway restart is enough to pick
// up template edits, which keeps YAML as the single source of truth.
func loadNPCTree(dir, npcID string) (*npcBehaviorTree, error) {
	if dir == "" {
		return nil, fmt.Errorf("npc template directory not configured")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read npc templates: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := filepath.Ext(entry.Name())
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		var raw rawNpcTemplate
		if err := yaml.Unmarshal(data, &raw); err != nil || raw.NPCID != npcID {
			continue
		}
		nodes := make(map[string]npcNode, len(raw.TalkTree.Nodes))
		for id, dto := range raw.TalkTree.Nodes {
			options := make([]npcOption, 0, len(dto.Options))
			for _, opt := range dto.Options {
				options = append(options, npcOption{ID: opt.ID, Text: opt.Text})
			}
			nodes[id] = npcNode{Say: dto.Say, Options: options}
		}
		return &npcBehaviorTree{
			NPCID:      raw.NPCID,
			Name:       raw.Name,
			Initial:    raw.TalkTree.Initial,
			DefaultSay: raw.TalkTree.DefaultSay,
			Nodes:      nodes,
		}, nil
	}
	return nil, fmt.Errorf("npc not found: %s", npcID)
}
