package httpgw

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
