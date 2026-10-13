package httpgw

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/aicity/a2a-gateway/internal/a2asrv"
)

func registerAgentForActions(t *testing.T, h http.Handler, agentID string) {
	t.Helper()
	w := doRequest(t, h, http.MethodPost, "/v1/cards", "", map[string]any{
		"agent_id":     agentID,
		"name":         agentID,
		"capabilities": []string{"dialogue"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("register status = %d, body=%s", w.Code, w.Body.String())
	}
}

func doOperatorRequest(t *testing.T, h http.Handler, method, path, key string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(data)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("X-Operator-Key", key)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestAgentActions_Move_ProxiesRegisteredAgent(t *testing.T) {
	var received map[string]any
	world := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/world/move" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode move: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"player_id":"alice","current_tile_id":"tile_1_0","x":150,"y":50,"ts_ms":1}`))
	}))
	defer world.Close()

	svc := newTestServer()
	srv := NewWithRuntime(svc, "", RuntimeConfig{WorldURL: world.URL})
	h := srv.Handler()
	registerAgentForActions(t, h, "alice")

	w := doRequest(t, h, http.MethodPost, "/v1/agent/actions/move", "", map[string]any{
		"agent_id": "alice",
		"tile_id":  "tile_1_0",
		"x":        150,
		"y":        50,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("move status = %d, body=%s", w.Code, w.Body.String())
	}
	if received["player_id"] != "alice" || received["to_tile_id"] != "tile_1_0" {
		t.Fatalf("proxy body = %#v", received)
	}
}

func TestAgentActions_Move_RejectsUnregisteredAgent(t *testing.T) {
	svc := newTestServer()
	srv := NewWithRuntime(svc, "", RuntimeConfig{WorldURL: "http://world.invalid"})
	h := srv.Handler()

	w := doRequest(t, h, http.MethodPost, "/v1/agent/actions/move", "", map[string]any{
		"agent_id": "ghost",
		"tile_id":  "tile_1_0",
	})
	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "F_005") {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestAgentActions_NPCBehaviorAndTalk(t *testing.T) {
	dir := t.TempDir()
	template := `
npc_id: npc_test_001
name: Test NPC
talk_tree:
  initial: root
  default_say: "default"
  nodes:
    root:
      say: "hello"
      options:
        - {id: food, text: "food"}
    food:
      say: "stew"
      options: []
`
	if err := os.WriteFile(filepath.Join(dir, "test.yaml"), []byte(template), 0o600); err != nil {
		t.Fatal(err)
	}

	svc := newTestServer()
	srv := NewWithRuntime(svc, "", RuntimeConfig{NPCTemplateDir: dir})
	h := srv.Handler()
	registerAgentForActions(t, h, "alice")

	w := doRequest(t, h, http.MethodGet, "/v1/agent/actions/npc-behavior?npc_id=npc_test_001", "", nil)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"initial":"root"`) {
		t.Fatalf("behavior status=%d body=%s", w.Code, w.Body.String())
	}

	w = doRequest(t, h, http.MethodPost, "/v1/agent/actions/npc-talk", "", map[string]any{
		"agent_id": "alice",
		"npc_id":   "npc_test_001",
		"node_id":  "",
	})
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"say":"hello"`) {
		t.Fatalf("talk status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestCORS_PreflightAllowedForCityUI(t *testing.T) {
	srv := New(newTestServer(), "")
	req := httptest.NewRequest(http.MethodOptions, "/v1/cards", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("preflight status=%d body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Fatalf("allow-origin=%q", got)
	}
}

func TestAgentActions_CreateNPCTemplate_DefaultDeny(t *testing.T) {
	store := a2asrv.NewMemoryTemplateStore(nil)
	svc := newTestServer()
	srv := NewWithRuntime(svc, "", RuntimeConfig{TemplateStore: store})
	h := srv.Handler()
	registerAgentForActions(t, h, "alice")

	w := doRequest(t, h, http.MethodPost, "/v1/agent/actions/npc-templates", "", map[string]any{
		"agent_id":     "alice",
		"name":         "Draft NPC",
		"home_tile_id": "tile_1_0",
		"personality":  map[string]any{"O": 0.5, "C": 0.5, "E": 0.5, "A": 0.5, "N": 0.5},
		"talk_tree":    map[string]any{"initial": "root", "nodes": map[string]any{"root": map[string]any{"say": "hello"}}},
	})
	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "F_017") {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	audits := store.Audits()
	if len(audits) != 1 || audits[0].Allowed {
		t.Fatalf("audits=%#v", audits)
	}
}

func TestAgentActions_CreateAndListNPCTemplate(t *testing.T) {
	store := a2asrv.NewMemoryTemplateStore(map[string]bool{"alice": true})
	svc := newTestServer()
	srv := NewWithRuntime(svc, "", RuntimeConfig{TemplateStore: store})
	h := srv.Handler()
	registerAgentForActions(t, h, "alice")
	registerAgentForActions(t, h, "bob")

	w := doRequest(t, h, http.MethodPost, "/v1/agent/actions/npc-templates", "", map[string]any{
		"agent_id":     "alice",
		"name":         "Draft NPC",
		"home_tile_id": "tile_1_0",
		"description":  "sandbox draft",
		"personality":  map[string]any{"O": 0.5, "C": 0.5, "E": 0.5, "A": 0.5, "N": 0.5},
		"talk_tree": map[string]any{
			"initial": "root",
			"nodes": map[string]any{
				"root": map[string]any{
					"say": "hello",
					"options": []any{map[string]any{
						"id": "stay", "text": "stay", "target": "terminal",
					}},
				},
				"terminal": map[string]any{"say": "bye"},
			},
		},
		"product_catalog": []any{map[string]any{"name": "stew", "price_gold": 10}},
	})
	if w.Code != http.StatusCreated || !strings.Contains(w.Body.String(), `"status":"draft"`) {
		t.Fatalf("create status=%d body=%s", w.Code, w.Body.String())
	}

	w = doRequest(t, h, http.MethodGet, "/v1/agent/actions/npc-templates?agent_id=alice", "", nil)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Draft NPC") {
		t.Fatalf("list status=%d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"valid":true`) {
		t.Fatalf("validation trace missing: %s", w.Body.String())
	}

	w = doRequest(t, h, http.MethodGet, "/v1/agent/actions/npc-templates?agent_id=bob", "", nil)
	if w.Code != http.StatusOK || strings.Contains(w.Body.String(), "Draft NPC") {
		t.Fatalf("other agent saw foreign draft: status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestAgentActions_EditorLoadAndSaveRevision(t *testing.T) {
	store := a2asrv.NewMemoryTemplateStore(map[string]bool{"alice": true})
	svc := newTestServer()
	srv := NewWithRuntime(svc, "", RuntimeConfig{TemplateStore: store})
	h := srv.Handler()
	registerAgentForActions(t, h, "alice")

	w := doRequest(t, h, http.MethodPost, "/v1/agent/actions/npc-templates", "", map[string]any{
		"agent_id": "alice", "name": "Editor NPC", "home_tile_id": "tile_1_0",
		"personality": map[string]any{"O": 0.5, "C": 0.5, "E": 0.5, "A": 0.5, "N": 0.5},
		"talk_tree": map[string]any{
			"initial": "root", "nodes": map[string]any{
				"root": map[string]any{
					"say": "hello",
					"options": []any{map[string]any{
						"id": "go", "text": "go", "target": "terminal",
					}},
				},
				"terminal": map[string]any{"say": "bye"},
			},
		},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", w.Code, w.Body.String())
	}
	var created struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &created)

	w = doRequest(t, h, http.MethodGet, fmt.Sprintf("/v1/agent/actions/npc-templates/%d/editor?agent_id=alice", created.ID), "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("editor load status=%d body=%s", w.Code, w.Body.String())
	}
	var loaded struct {
		Version int64          `json:"version"`
		BTTree  map[string]any `json:"bt_editor_tree"`
		Edges   []any          `json:"node_edges"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &loaded); err != nil {
		t.Fatalf("decode editor load: %v", err)
	}
	if loaded.Version != 1 || loaded.BTTree["type"] != "sequence" || len(loaded.Edges) != 1 {
		t.Fatalf("editor load=%#v", loaded)
	}

	savedTree := map[string]any{
		"initial": "root", "nodes": map[string]any{
			"root": map[string]any{
				"say": "edited hello",
				"options": []any{map[string]any{
					"id": "go", "text": "go", "target": "terminal",
				}},
			},
			"terminal": map[string]any{"say": "bye"},
		},
	}
	w = doRequest(t, h, http.MethodPut, fmt.Sprintf("/v1/agent/actions/npc-templates/%d/editor", created.ID), "", map[string]any{
		"agent_id": "alice", "talk_tree": savedTree,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("editor save status=%d body=%s", w.Code, w.Body.String())
	}
	var saved struct {
		Version  int64          `json:"version"`
		Status   string         `json:"status"`
		TalkTree map[string]any `json:"talk_tree"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &saved); err != nil {
		t.Fatalf("decode editor save: %v", err)
	}
	if saved.Version != 2 || saved.Status != "draft" {
		t.Fatalf("editor save=%#v", saved)
	}
	nodes := saved.TalkTree["nodes"].(map[string]any)
	root := nodes["root"].(map[string]any)
	if root["say"] != "edited hello" {
		t.Fatalf("saved tree root=%#v", root)
	}
}

func TestAgentActions_EditorSaveEnforcesOwnerAndState(t *testing.T) {
	store := a2asrv.NewMemoryTemplateStore(map[string]bool{"alice": true})
	svc := newTestServer()
	srv := NewWithRuntime(svc, "", RuntimeConfig{TemplateStore: store})
	h := srv.Handler()
	registerAgentForActions(t, h, "alice")
	registerAgentForActions(t, h, "bob")

	w := doRequest(t, h, http.MethodPost, "/v1/agent/actions/npc-templates", "", map[string]any{
		"agent_id": "alice", "name": "Owned NPC", "home_tile_id": "tile_1_0",
		"personality": map[string]any{"O": 0.5, "C": 0.5, "E": 0.5, "A": 0.5, "N": 0.5},
		"talk_tree": map[string]any{
			"initial": "root", "nodes": map[string]any{
				"root": map[string]any{"say": "hello"},
			},
		},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", w.Code, w.Body.String())
	}
	var created struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	tree := map[string]any{
		"initial": "root", "nodes": map[string]any{
			"root": map[string]any{"say": "hijack"},
		},
	}
	w = doRequest(t, h, http.MethodPut, fmt.Sprintf("/v1/agent/actions/npc-templates/%d/editor", created.ID), "", map[string]any{
		"agent_id": "bob", "talk_tree": tree,
	})
	if w.Code != http.StatusNotFound {
		t.Fatalf("foreign save status=%d body=%s", w.Code, w.Body.String())
	}

	w = doRequest(t, h, http.MethodPost, fmt.Sprintf("/v1/agent/actions/npc-templates/%d/submit", created.ID), "", map[string]any{
		"agent_id": "alice",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("submit status=%d body=%s", w.Code, w.Body.String())
	}
	w = doRequest(t, h, http.MethodPut, fmt.Sprintf("/v1/agent/actions/npc-templates/%d/editor", created.ID), "", map[string]any{
		"agent_id": "alice", "talk_tree": tree,
	})
	if w.Code != http.StatusConflict {
		t.Fatalf("submitted save status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestAgentActions_CreateNPCTemplate_RejectsTalkTreeCycle(t *testing.T) {
	store := a2asrv.NewMemoryTemplateStore(map[string]bool{"alice": true})
	svc := newTestServer()
	srv := NewWithRuntime(svc, "", RuntimeConfig{TemplateStore: store})
	h := srv.Handler()
	registerAgentForActions(t, h, "alice")

	w := doRequest(t, h, http.MethodPost, "/v1/agent/actions/npc-templates", "", map[string]any{
		"agent_id":     "alice",
		"name":         "Looping NPC",
		"home_tile_id": "tile_1_0",
		"personality":  map[string]any{"O": 0.5, "C": 0.5, "E": 0.5, "A": 0.5, "N": 0.5},
		"talk_tree": map[string]any{
			"initial": "root",
			"nodes": map[string]any{
				"root": map[string]any{
					"say": "hello",
					"options": []any{map[string]any{
						"id": "loop", "text": "loop", "target": "root",
					}},
				},
			},
		},
	})
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "cycle") {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestAgentActions_CreateNPCTemplate_ValidationFails(t *testing.T) {
	store := a2asrv.NewMemoryTemplateStore(map[string]bool{"alice": true})
	svc := newTestServer()
	srv := NewWithRuntime(svc, "", RuntimeConfig{TemplateStore: store})
	h := srv.Handler()
	registerAgentForActions(t, h, "alice")

	w := doRequest(t, h, http.MethodPost, "/v1/agent/actions/npc-templates", "", map[string]any{
		"agent_id":     "alice",
		"name":         "Bad NPC",
		"home_tile_id": "tile_1_0",
		"personality":  map[string]any{"O": 1.5, "C": 0.5, "E": 0.5, "A": 0.5, "N": 0.5},
		"talk_tree":    map[string]any{"initial": "root", "nodes": map[string]any{"root": map[string]any{"say": "hello"}}},
	})
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "R_019") {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if drafts, _ := store.ListDrafts(nil, "alice", 10); len(drafts) != 0 {
		t.Fatalf("invalid draft persisted")
	}
}

func TestAgentActions_SubmitApproveAndSimulateNPCTemplate(t *testing.T) {
	store := a2asrv.NewMemoryTemplateStore(map[string]bool{"alice": true})
	svc := newTestServer()
	srv := NewWithRuntime(svc, "", RuntimeConfig{
		TemplateStore:  store,
		OperatorAPIKey: "operator-secret",
	})
	h := srv.Handler()
	registerAgentForActions(t, h, "alice")

	w := doRequest(t, h, http.MethodPost, "/v1/agent/actions/npc-templates", "", map[string]any{
		"agent_id":     "alice",
		"name":         "Review NPC",
		"home_tile_id": "tile_1_0",
		"personality":  map[string]any{"O": 0.5, "C": 0.5, "E": 0.5, "A": 0.5, "N": 0.5},
		"talk_tree": map[string]any{
			"initial": "root",
			"nodes": map[string]any{
				"root": map[string]any{
					"say": "hello",
					"options": []any{map[string]any{
						"id": "go", "text": "go", "target": "terminal",
					}},
				},
				"terminal": map[string]any{"say": "bye"},
			},
		},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", w.Code, w.Body.String())
	}
	var created struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	resource := strconv.FormatInt(created.ID, 10)

	w = doRequest(t, h, http.MethodPost, "/v1/agent/actions/npc-templates/"+resource+"/submit", "", map[string]any{
		"agent_id": "alice",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("submit status=%d body=%s", w.Code, w.Body.String())
	}

	w = doOperatorRequest(t, h, http.MethodGet, "/v1/operator/npc-templates?status=submitted", "operator-secret", nil)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Review NPC") {
		t.Fatalf("review list status=%d body=%s", w.Code, w.Body.String())
	}

	w = doOperatorRequest(t, h, http.MethodPost, "/v1/operator/npc-templates/"+resource+"/approve", "operator-secret", map[string]any{
		"reviewer": "ops-one",
	})
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"status":"approved"`) {
		t.Fatalf("approve status=%d body=%s", w.Code, w.Body.String())
	}

	w = doRequest(t, h, http.MethodPost, "/v1/agent/actions/npc-templates/"+resource+"/simulate", "", map[string]any{
		"agent_id": "alice",
		"choices":  []string{"go"},
	})
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"status":"completed"`) {
		t.Fatalf("simulate status=%d body=%s", w.Code, w.Body.String())
	}

	draft, err := store.GetDraft(nil, created.ID)
	if err != nil {
		t.Fatalf("get draft: %v", err)
	}
	if draft.SimulationTrace["sandbox"] != true || draft.Status != "approved" {
		t.Fatalf("draft after simulate = %#v", draft)
	}
	audits := store.Audits()
	foundApproval := false
	for _, audit := range audits {
		if audit.Action == "npc_template.approved" && audit.ResourceID == resource {
			foundApproval = true
		}
	}
	if !foundApproval {
		t.Fatalf("approval audit missing: %#v", audits)
	}
}
