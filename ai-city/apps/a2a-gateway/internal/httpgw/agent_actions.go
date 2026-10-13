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
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"

	"github.com/aicity/a2a-gateway/internal/a2asrv"
)

// RuntimeConfig points the HTTP gateway at local world services and the
// shared NPC templates. Empty WorldURL disables movement actions.
type RuntimeConfig struct {
	WorldURL       string
	NPCTemplateDir string
	TemplateStore  a2asrv.TemplateStore
	OperatorAPIKey string
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

type npcTemplateRequest struct {
	AgentID        string             `json:"agent_id"`
	Name           string             `json:"name"`
	HomeTileID     string             `json:"home_tile_id"`
	Description    string             `json:"description"`
	Personality    map[string]float64 `json:"personality"`
	TalkTree       map[string]any     `json:"talk_tree"`
	ProductCatalog []map[string]any   `json:"product_catalog"`
}

type npcTemplateCreatedResponse struct {
	ID     int64  `json:"id"`
	Status string `json:"status"`
}

type npcTemplateDraftResponse struct {
	ID              int64              `json:"id"`
	AgentID         string             `json:"agent_id"`
	Name            string             `json:"name"`
	HomeTileID      string             `json:"home_tile_id"`
	Description     string             `json:"description"`
	Personality     map[string]float64 `json:"personality"`
	TalkTree        map[string]any     `json:"talk_tree"`
	ProductCatalog  []map[string]any   `json:"product_catalog"`
	Status          string             `json:"status"`
	ValidationTrace map[string]any     `json:"validation_trace"`
	SimulationTrace map[string]any     `json:"simulation_trace"`
	ReviewedBy      string             `json:"reviewed_by"`
	ReviewReason    string             `json:"review_reason"`
}

type npcSimulationRequest struct {
	AgentID   string   `json:"agent_id"`
	StartNode string   `json:"start_node"`
	Choices   []string `json:"choices"`
}

type npcSimulationStep struct {
	Index          int    `json:"index"`
	NodeID         string `json:"node_id"`
	Say            string `json:"say"`
	SelectedOption string `json:"selected_option_id,omitempty"`
	TargetNodeID   string `json:"target_node_id,omitempty"`
}

type npcSimulationResponse struct {
	TemplateID int64               `json:"template_id"`
	Status     string              `json:"status"`
	Steps      []npcSimulationStep `json:"steps"`
	Error      string              `json:"error,omitempty"`
}

type npcReviewRequest struct {
	Reviewer string `json:"reviewer"`
	Reason   string `json:"reason"`
}

type npcPermissionRequest struct {
	CanCreateNPC bool `json:"can_create_npc"`
	DailyQuota   int  `json:"daily_quota"`
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

// CreateNPCTemplate creates a sandbox draft owned by the calling Agent.
// A draft never mutates runtime NPC YAML or enters the live marketplace.
func (s *Server) CreateNPCTemplate(c *gin.Context) {
	traceID := traceIDFromCtx(c)
	var req npcTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorEnvelope("F_001", "bad_request:"+err.Error(), traceID))
		return
	}
	if req.AgentID == "" || req.Name == "" || req.HomeTileID == "" {
		c.JSON(http.StatusBadRequest, errorEnvelope("F_001", "agent_id, name and home_tile_id required", traceID))
		return
	}
	if !s.svc.HasAgent(req.AgentID) {
		c.JSON(http.StatusForbidden, errorEnvelope("F_005", "agent not registered", traceID))
		return
	}
	if s.templateStore == nil {
		c.JSON(http.StatusServiceUnavailable, errorEnvelope("R_016", "template store not configured", traceID))
		return
	}

	allowed, err := s.templateStore.CanCreateNPC(c.Request.Context(), req.AgentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorEnvelope("F_INTERNAL", err.Error(), traceID))
		return
	}
	if !allowed {
		_ = s.templateStore.Audit(c.Request.Context(), a2asrv.AgentActionAudit{
			AgentID: req.AgentID, Action: "npc_template.create", Allowed: false,
			Reason: "permission denied", TraceID: traceID,
			Metadata: map[string]any{"name": req.Name},
		})
		c.JSON(http.StatusForbidden, errorEnvelope("F_017", "agent may not create NPC templates", traceID))
		return
	}

	trace, err := validateNPCDraft(req.Name, req.Personality, req.TalkTree, req.ProductCatalog)
	if err != nil {
		_ = s.templateStore.Audit(c.Request.Context(), a2asrv.AgentActionAudit{
			AgentID: req.AgentID, Action: "npc_template.create", Allowed: false,
			Reason: err.Error(), TraceID: traceID,
			Metadata: map[string]any{"name": req.Name},
		})
		c.JSON(http.StatusBadRequest, errorEnvelope("R_019", err.Error(), traceID))
		return
	}

	draft := a2asrv.AgentTemplateDraft{
		AgentID: req.AgentID, Name: req.Name, HomeTileID: req.HomeTileID,
		Description: req.Description, Personality: req.Personality,
		TalkTree: req.TalkTree, ProductCatalog: req.ProductCatalog,
		Status: "draft", ValidationTrace: trace,
	}
	id, err := s.templateStore.CreateDraft(c.Request.Context(), draft)
	if err != nil {
		if err == a2asrv.ErrTemplateQuotaExceeded {
			c.JSON(http.StatusTooManyRequests, errorEnvelope("R_021", "agent daily template quota exceeded", traceID))
			return
		}
		c.JSON(http.StatusInternalServerError, errorEnvelope("F_INTERNAL", err.Error(), traceID))
		return
	}
	if err := s.templateStore.Audit(c.Request.Context(), a2asrv.AgentActionAudit{
		AgentID: req.AgentID, Action: "npc_template.create", ResourceID: fmt.Sprintf("%d", id),
		Allowed: true, TraceID: traceID,
		Metadata: map[string]any{"name": req.Name, "nodes": trace["nodes"]},
	}); err != nil {
		c.JSON(http.StatusInternalServerError, errorEnvelope("F_INTERNAL", err.Error(), traceID))
		return
	}
	c.JSON(http.StatusCreated, npcTemplateCreatedResponse{ID: id, Status: "draft"})
}

// ListNPCTemplates returns only the caller's sandbox drafts.
func (s *Server) ListNPCTemplates(c *gin.Context) {
	traceID := traceIDFromCtx(c)
	agentID := c.Query("agent_id")
	if agentID == "" {
		c.JSON(http.StatusBadRequest, errorEnvelope("F_001", "agent_id required", traceID))
		return
	}
	if !s.svc.HasAgent(agentID) {
		c.JSON(http.StatusForbidden, errorEnvelope("F_005", "agent not registered", traceID))
		return
	}
	if s.templateStore == nil {
		c.JSON(http.StatusServiceUnavailable, errorEnvelope("R_016", "template store not configured", traceID))
		return
	}
	limit := 20
	if value := c.Query("limit"); value != "" {
		n, err := strconv.Atoi(value)
		if err != nil || n < 1 || n > 100 {
			c.JSON(http.StatusBadRequest, errorEnvelope("F_014", "limit out of range [1, 100]", traceID))
			return
		}
		limit = n
	}
	drafts, err := s.templateStore.ListDrafts(c.Request.Context(), agentID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorEnvelope("F_INTERNAL", err.Error(), traceID))
		return
	}
	response := make([]npcTemplateDraftResponse, 0, len(drafts))
	for _, draft := range drafts {
		response = append(response, npcTemplateDraftResponse{
			ID: draft.ID, AgentID: draft.AgentID, Name: draft.Name,
			HomeTileID: draft.HomeTileID, Description: draft.Description,
			Personality: draft.Personality, TalkTree: draft.TalkTree,
			ProductCatalog: draft.ProductCatalog, Status: draft.Status,
			ValidationTrace: draft.ValidationTrace, SimulationTrace: draft.SimulationTrace,
			ReviewedBy: draft.ReviewedBy, ReviewReason: draft.ReviewReason,
		})
	}
	c.JSON(http.StatusOK, gin.H{"drafts": response})
}

func parseTemplateID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		c.JSON(http.StatusBadRequest, errorEnvelope("F_001", "template id invalid", traceIDFromCtx(c)))
		return 0, false
	}
	return id, true
}

func requireTemplateOperator(s *Server, c *gin.Context) bool {
	if s.operatorKey == "" {
		c.JSON(http.StatusServiceUnavailable, errorEnvelope("R_016", "operator api not configured", traceIDFromCtx(c)))
		return false
	}
	if c.GetHeader("X-Operator-Key") != s.operatorKey {
		c.JSON(http.StatusUnauthorized, errorEnvelope("F_AUTH", "invalid operator key", traceIDFromCtx(c)))
		return false
	}
	return true
}

func templateDraftResponses(drafts []a2asrv.AgentTemplateDraft) []npcTemplateDraftResponse {
	response := make([]npcTemplateDraftResponse, 0, len(drafts))
	for _, draft := range drafts {
		response = append(response, npcTemplateDraftResponse{
			ID: draft.ID, AgentID: draft.AgentID, Name: draft.Name,
			HomeTileID: draft.HomeTileID, Description: draft.Description,
			Personality: draft.Personality, TalkTree: draft.TalkTree,
			ProductCatalog: draft.ProductCatalog, Status: draft.Status,
			ValidationTrace: draft.ValidationTrace, SimulationTrace: draft.SimulationTrace,
			ReviewedBy: draft.ReviewedBy, ReviewReason: draft.ReviewReason,
		})
	}
	return response
}

func (s *Server) SubmitNPCTemplate(c *gin.Context) {
	traceID := traceIDFromCtx(c)
	templateID, ok := parseTemplateID(c)
	if !ok {
		return
	}
	var req struct {
		AgentID string `json:"agent_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.AgentID == "" {
		c.JSON(http.StatusBadRequest, errorEnvelope("F_001", "agent_id required", traceID))
		return
	}
	if !s.svc.HasAgent(req.AgentID) || s.templateStore == nil {
		status := http.StatusForbidden
		message := "agent not registered"
		if s.templateStore == nil {
			status = http.StatusServiceUnavailable
			message = "template store not configured"
		}
		c.JSON(status, errorEnvelope("F_005", message, traceID))
		return
	}
	if err := s.templateStore.SubmitDraft(c.Request.Context(), templateID, req.AgentID); err != nil {
		if err == a2asrv.ErrTemplateState {
			c.JSON(http.StatusConflict, errorEnvelope("R_030", "draft cannot be submitted", traceID))
			return
		}
		c.JSON(http.StatusInternalServerError, errorEnvelope("F_INTERNAL", err.Error(), traceID))
		return
	}
	_ = s.templateStore.Audit(c.Request.Context(), a2asrv.AgentActionAudit{
		AgentID: req.AgentID, Action: "npc_template.submit",
		ResourceID: strconv.FormatInt(templateID, 10), Allowed: true, TraceID: traceID,
	})
	c.JSON(http.StatusOK, gin.H{"id": templateID, "status": "submitted"})
}

func (s *Server) SimulateNPCTemplate(c *gin.Context) {
	traceID := traceIDFromCtx(c)
	templateID, ok := parseTemplateID(c)
	if !ok {
		return
	}
	var req npcSimulationRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.AgentID == "" {
		c.JSON(http.StatusBadRequest, errorEnvelope("F_001", "agent_id required", traceID))
		return
	}
	if !s.svc.HasAgent(req.AgentID) || s.templateStore == nil {
		status := http.StatusForbidden
		message := "agent not registered"
		if s.templateStore == nil {
			status = http.StatusServiceUnavailable
			message = "template store not configured"
		}
		c.JSON(status, errorEnvelope("F_005", message, traceID))
		return
	}
	draft, err := s.templateStore.GetDraft(c.Request.Context(), templateID)
	if err != nil {
		status := http.StatusInternalServerError
		if err == a2asrv.ErrTemplateNotFound {
			status = http.StatusNotFound
		}
		c.JSON(status, errorEnvelope("R_028", err.Error(), traceID))
		return
	}
	if draft.AgentID != req.AgentID {
		c.JSON(http.StatusNotFound, errorEnvelope("R_028", "template draft not found", traceID))
		return
	}
	nodesRaw, nodesOK := draft.TalkTree["nodes"].(map[string]any)
	initialRaw, initialOK := draft.TalkTree["initial"].(string)
	if !nodesOK || !initialOK || initialRaw == "" {
		c.JSON(http.StatusBadRequest, errorEnvelope("R_019", "draft talk tree is invalid", traceID))
		return
	}

	current := req.StartNode
	if current == "" {
		current = initialRaw
	}
	steps := []npcSimulationStep{}
	simulationStatus := "completed"
	simulationError := ""
	choiceIndex := 0
	for index := 0; index < 20; index++ {
		nodeRaw, nodeOK := nodesRaw[current].(map[string]any)
		if !nodeOK {
			simulationStatus = "failed"
			simulationError = "node not found: " + current
			break
		}
		say, _ := nodeRaw["say"].(string)
		options, _ := nodeRaw["options"].([]any)
		if len(options) == 0 {
			steps = append(steps, npcSimulationStep{Index: index, NodeID: current, Say: say})
			break
		}
		if choiceIndex >= len(req.Choices) {
			simulationStatus = "waiting_choice"
			steps = append(steps, npcSimulationStep{Index: index, NodeID: current, Say: say})
			break
		}
		selected := req.Choices[choiceIndex]
		choiceIndex++
		target := ""
		found := false
		for _, optionRaw := range options {
			option, optionOK := optionRaw.(map[string]any)
			if !optionOK {
				continue
			}
			optionID, _ := option["id"].(string)
			if optionID == selected {
				target, _ = option["target"].(string)
				found = true
				break
			}
		}
		if !found || target == "" {
			simulationStatus = "failed"
			simulationError = "invalid option: " + selected
			break
		}
		steps = append(steps, npcSimulationStep{
			Index: index, NodeID: current, Say: say,
			SelectedOption: selected, TargetNodeID: target,
		})
		current = target
	}

	trace := map[string]any{
		"sandbox": true, "status": simulationStatus, "steps": steps,
		"start_node": req.StartNode, "choices": req.Choices,
	}
	if simulationError != "" {
		trace["error"] = simulationError
	}
	if err := s.templateStore.SaveSimulationTrace(c.Request.Context(), templateID, req.AgentID, trace); err != nil {
		c.JSON(http.StatusInternalServerError, errorEnvelope("F_INTERNAL", err.Error(), traceID))
		return
	}
	_ = s.templateStore.Audit(c.Request.Context(), a2asrv.AgentActionAudit{
		AgentID: req.AgentID, Action: "npc_template.simulate",
		ResourceID: strconv.FormatInt(templateID, 10), Allowed: true, TraceID: traceID,
		Metadata: map[string]any{"status": simulationStatus, "steps": len(steps)},
	})
	c.JSON(http.StatusOK, npcSimulationResponse{
		TemplateID: templateID, Status: simulationStatus, Steps: steps, Error: simulationError,
	})
}

func (s *Server) ListNPCTemplatesForReview(c *gin.Context) {
	traceID := traceIDFromCtx(c)
	if !requireTemplateOperator(s, c) {
		return
	}
	if s.templateStore == nil {
		c.JSON(http.StatusServiceUnavailable, errorEnvelope("R_016", "template store not configured", traceID))
		return
	}
	status := c.DefaultQuery("status", "submitted")
	if status != "draft" && status != "submitted" && status != "approved" && status != "rejected" {
		c.JSON(http.StatusBadRequest, errorEnvelope("F_001", "status invalid", traceID))
		return
	}
	drafts, err := s.templateStore.ListDraftsByStatus(c.Request.Context(), status, 100)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorEnvelope("F_INTERNAL", err.Error(), traceID))
		return
	}
	c.JSON(http.StatusOK, gin.H{"drafts": templateDraftResponses(drafts), "trace_id": traceID})
}

func (s *Server) ApproveNPCTemplate(c *gin.Context) {
	s.reviewNPCTemplate(c, "approved")
}

func (s *Server) RejectNPCTemplate(c *gin.Context) {
	s.reviewNPCTemplate(c, "rejected")
}

func (s *Server) reviewNPCTemplate(c *gin.Context, status string) {
	traceID := traceIDFromCtx(c)
	if !requireTemplateOperator(s, c) {
		return
	}
	templateID, ok := parseTemplateID(c)
	if !ok {
		return
	}
	if s.templateStore == nil {
		c.JSON(http.StatusServiceUnavailable, errorEnvelope("R_016", "template store not configured", traceID))
		return
	}
	var req npcReviewRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Reviewer == "" {
		c.JSON(http.StatusBadRequest, errorEnvelope("F_001", "reviewer required", traceID))
		return
	}
	if status == "rejected" && req.Reason == "" {
		c.JSON(http.StatusBadRequest, errorEnvelope("F_001", "reject reason required", traceID))
		return
	}
	draft, err := s.templateStore.GetDraft(c.Request.Context(), templateID)
	if err != nil {
		httpStatus := http.StatusInternalServerError
		if err == a2asrv.ErrTemplateNotFound {
			httpStatus = http.StatusNotFound
		}
		c.JSON(httpStatus, errorEnvelope("R_028", err.Error(), traceID))
		return
	}
	if err := s.templateStore.ReviewDraft(c.Request.Context(), templateID, status, req.Reviewer, req.Reason); err != nil {
		if err == a2asrv.ErrTemplateState {
			c.JSON(http.StatusConflict, errorEnvelope("R_030", "draft is not submitted", traceID))
			return
		}
		c.JSON(http.StatusInternalServerError, errorEnvelope("F_INTERNAL", err.Error(), traceID))
		return
	}
	_ = s.templateStore.Audit(c.Request.Context(), a2asrv.AgentActionAudit{
		AgentID: draft.AgentID, Action: "npc_template." + status,
		ResourceID: strconv.FormatInt(templateID, 10), Allowed: true,
		Reason: req.Reason, TraceID: traceID,
		Metadata: map[string]any{"reviewer": req.Reviewer},
	})
	c.JSON(http.StatusOK, gin.H{"id": templateID, "status": status})
}

func (s *Server) GrantAgentTemplatePermission(c *gin.Context) {
	traceID := traceIDFromCtx(c)
	if !requireTemplateOperator(s, c) {
		return
	}
	if s.templateStore == nil {
		c.JSON(http.StatusServiceUnavailable, errorEnvelope("R_016", "template store not configured", traceID))
		return
	}
	agentID := c.Param("agent_id")
	if agentID == "" {
		c.JSON(http.StatusBadRequest, errorEnvelope("F_001", "agent_id required", traceID))
		return
	}
	var req npcPermissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorEnvelope("F_001", "bad_request:"+err.Error(), traceID))
		return
	}
	quota := req.DailyQuota
	if quota == 0 {
		quota = 10
	}
	if quota < 1 || quota > 1000 {
		c.JSON(http.StatusBadRequest, errorEnvelope("F_001", "daily_quota out of range [1, 1000]", traceID))
		return
	}
	permission := a2asrv.AgentTemplatePermission{
		AgentID: agentID, CanCreateNPC: req.CanCreateNPC, DailyQuota: quota,
		GrantedBy: "operator", Reason: "M2 template authoring",
	}
	if err := s.templateStore.GrantPermission(c.Request.Context(), permission); err != nil {
		c.JSON(http.StatusInternalServerError, errorEnvelope("F_INTERNAL", err.Error(), traceID))
		return
	}
	_ = s.templateStore.Audit(c.Request.Context(), a2asrv.AgentActionAudit{
		AgentID: agentID, Action: "npc_template.permission.grant", Allowed: true,
		TraceID: traceID, Metadata: map[string]any{"can_create": req.CanCreateNPC, "quota": quota},
	})
	c.JSON(http.StatusOK, gin.H{"agent_id": agentID, "can_create_npc": req.CanCreateNPC, "daily_quota": quota})
}

func validateNPCDraft(name string, personality map[string]float64,
	talkTree map[string]any, catalog []map[string]any,
) (map[string]any, error) {
	if utf8.RuneCountInString(name) > 64 {
		return nil, fmt.Errorf("name exceeds 64 characters")
	}
	for key, min := range map[string]float64{"O": 0, "C": 0, "E": 0, "A": 0, "N": 0} {
		value, ok := personality[key]
		if !ok || value < min || value > 1 {
			return nil, fmt.Errorf("personality.%s must be in [0, 1]", key)
		}
	}
	if len(talkTree) == 0 {
		return nil, fmt.Errorf("talk_tree is required")
	}

	initialRaw, ok := talkTree["initial"]
	initial, _ := initialRaw.(string)
	if !ok || initial == "" {
		return nil, fmt.Errorf("talk_tree.initial is required")
	}
	nodesRaw, nodesFound := talkTree["nodes"]
	nodes, nodesOK := nodesRaw.(map[string]any)
	if !nodesFound || !nodesOK || len(nodes) == 0 {
		return nil, fmt.Errorf("talk_tree.nodes is required")
	}
	if _, ok := nodes[initial]; !ok {
		return nil, fmt.Errorf("talk_tree.initial node not found: %s", initial)
	}
	if len(nodes) > 50 {
		return nil, fmt.Errorf("talk_tree exceeds 50 nodes")
	}

	optionCount := 0
	for nodeID, nodeRaw := range nodes {
		node, ok := nodeRaw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("talk_tree.nodes.%s must be an object", nodeID)
		}
		sayRaw, sayOK := node["say"]
		say, sayString := sayRaw.(string)
		if !sayOK || !sayString || say == "" {
			return nil, fmt.Errorf("talk_tree.nodes.%s.say is required", nodeID)
		}
		optionsRaw := node["options"]
		if optionsRaw == nil {
			continue
		}
		options, optionsListOK := optionsRaw.([]any)
		if !optionsListOK || len(options) == 0 {
			return nil, fmt.Errorf("talk_tree.nodes.%s.options must be a non-empty list", nodeID)
		}
		optionIDs := map[string]bool{}
		for _, optionRaw := range options {
			optionCount++
			option, ok := optionRaw.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("talk_tree.nodes.%s option must be an object", nodeID)
			}
			idRaw, idOK := option["id"]
			optionID, idString := idRaw.(string)
			if !idOK || !idString || optionID == "" || optionIDs[optionID] {
				return nil, fmt.Errorf("talk_tree.nodes.%s option id is required and unique", nodeID)
			}
			textRaw, textOK := option["text"]
			optionText, textString := textRaw.(string)
			if !textOK || !textString || optionText == "" {
				return nil, fmt.Errorf("talk_tree.nodes.%s option text is required", nodeID)
			}
			targetRaw, targetOK := option["target"]
			target, targetString := targetRaw.(string)
			if !targetOK || !targetString {
				return nil, fmt.Errorf("talk_tree.nodes.%s option target is required", nodeID)
			}
			if _, ok := nodes[target]; !ok {
				return nil, fmt.Errorf("talk_tree target not found: %s", target)
			}
			optionIDs[optionID] = true
		}
	}
	if len(catalog) > 20 {
		return nil, fmt.Errorf("product_catalog exceeds 20 items")
	}
	for index, item := range catalog {
		nameRaw, nameOK := item["name"]
		itemName, nameString := nameRaw.(string)
		if !nameOK || !nameString || itemName == "" {
			return nil, fmt.Errorf("product_catalog.%d.name is required", index)
		}
		priceRaw, priceOK := item["price_gold"]
		price, priceNumber := priceRaw.(float64)
		if !priceOK || !priceNumber || price < 0 || price != float64(int(price)) {
			return nil, fmt.Errorf("product_catalog.%d.price_gold must be a non-negative integer", index)
		}
	}
	depthCache := map[string]int{}
	visiting := map[string]bool{}
	var walkDepth func(nodeID string) (int, error)
	walkDepth = func(nodeID string) (int, error) {
		if depth, ok := depthCache[nodeID]; ok {
			return depth, nil
		}
		if visiting[nodeID] {
			return 0, fmt.Errorf("talk_tree cycle at node: %s", nodeID)
		}
		if _, ok := nodes[nodeID]; !ok {
			return 0, fmt.Errorf("talk_tree target not found: %s", nodeID)
		}
		visiting[nodeID] = true
		defer delete(visiting, nodeID)

		node := nodes[nodeID].(map[string]any)
		maxChildDepth := 0
		if optionsRaw, ok := node["options"]; ok && optionsRaw != nil {
			for _, optionRaw := range optionsRaw.([]any) {
				option := optionRaw.(map[string]any)
				target := option["target"].(string)
				childDepth, err := walkDepth(target)
				if err != nil {
					return 0, err
				}
				if childDepth > maxChildDepth {
					maxChildDepth = childDepth
				}
			}
		}
		depth := 1 + maxChildDepth
		if depth > 10 {
			return 0, fmt.Errorf("talk_tree exceeds max depth of 10")
		}
		depthCache[nodeID] = depth
		return depth, nil
	}
	treeDepth, err := walkDepth(initial)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"valid": true, "nodes": len(nodes), "options": optionCount,
		"initial": initial, "depth": treeDepth,
	}, nil
}
