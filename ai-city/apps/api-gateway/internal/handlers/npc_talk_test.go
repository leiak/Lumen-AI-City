package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aicity/api-gateway/internal/npc"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type fakeRedis struct {
	published []struct {
		Channel string
		Payload string
	}
}

func (f *fakeRedis) Publish(_ context.Context, channel, payload string) *redis.IntCmd {
	f.published = append(f.published, struct {
		Channel string
		Payload string
	}{channel, payload})
	cmd := redis.NewIntCmd(context.Background())
	cmd.SetVal(int64(1))
	return cmd
}

func setupTestRouter(t *testing.T, trees map[string]*npc.Tree) (*gin.Engine, *fakeRedis) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	fakeR := &fakeRedis{}
	h := NewNPCTalkHandler(trees, fakeR, zap.NewNop(), "aicity:npc_dialogue")
	r.POST("/v1/npc/talk", h.Handle)
	return r, fakeR
}

func TestNPCTalk_HappyPath(t *testing.T) {
	trees := map[string]*npc.Tree{
		"npc_wang_boss_001": {
			NpcID:    "npc_wang_boss_001",
			HomeTile: "tile_1_1",
			Nodes: map[string]npc.Node{
				"ask_business": {
					Say: "小店经营杂货。",
					Options: []npc.Option{
						{ID: "yes_browse", Text: "我看看"},
					},
				},
			},
		},
	}
	r, fakeR := setupTestRouter(t, trees)
	body := bytes.NewBufferString(`{"npc_id":"npc_wang_boss_001","player_id":"player-1","choice_id":"ask_business"}`)
	req := httptest.NewRequest("POST", "/v1/npc/talk", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, w.Body.String())
	}
	if resp["say"] != "小店经营杂货。" {
		t.Errorf("say = %v", resp["say"])
	}
	if resp["reply_to_choice_id"] != "ask_business" {
		t.Errorf("reply_to_choice_id = %v", resp["reply_to_choice_id"])
	}
	if resp["tile_id"] != "tile_1_1" {
		t.Errorf("tile_id = %v", resp["tile_id"])
	}
	if len(fakeR.published) != 1 {
		t.Errorf("expected 1 publish, got %d", len(fakeR.published))
	}
	if fakeR.published[0].Channel != "aicity:npc_dialogue" {
		t.Errorf("channel = %s", fakeR.published[0].Channel)
	}

	// Published payload must be the INNER payload (no outer envelope).
	// ws-gateway wraps it uniformly — publishers must NOT double-wrap.
	var published map[string]any
	if err := json.Unmarshal([]byte(fakeR.published[0].Payload), &published); err != nil {
		t.Fatalf("unmarshal published payload: %v raw=%s", err, fakeR.published[0].Payload)
	}
	if _, hasType := published["type"]; hasType {
		t.Errorf("published payload should NOT have outer 'type' key; got %+v", published)
	}
	if _, hasPayload := published["payload"]; hasPayload {
		t.Errorf("published payload should NOT have outer 'payload' key; got %+v", published)
	}
	if published["npc_id"] != "npc_wang_boss_001" {
		t.Errorf("published npc_id = %v, want npc_wang_boss_001", published["npc_id"])
	}
	if published["say"] != "小店经营杂货。" {
		t.Errorf("published say = %v, want 小店经营杂货。", published["say"])
	}
	if published["reply_to_choice_id"] != "ask_business" {
		t.Errorf("published reply_to_choice_id = %v, want ask_business", published["reply_to_choice_id"])
	}
	if published["tile_id"] != "tile_1_1" {
		t.Errorf("published tile_id = %v, want tile_1_1", published["tile_id"])
	}
	if _, ok := published["ts_ms"].(float64); !ok {
		t.Errorf("published ts_ms should be a number; got %T (%v)", published["ts_ms"], published["ts_ms"])
	}
	if _, ok := published["trace_id"].(string); !ok {
		t.Errorf("published trace_id should be a string; got %T (%v)", published["trace_id"], published["trace_id"])
	}
}

func TestNPCTalk_NPCNotFound(t *testing.T) {
	r, _ := setupTestRouter(t, map[string]*npc.Tree{})
	body := bytes.NewBufferString(`{"npc_id":"npc_ghost","player_id":"p","choice_id":"x"}`)
	req := httptest.NewRequest("POST", "/v1/npc/talk", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "NPC_001" {
		t.Errorf("error = %v, want NPC_001", resp["error"])
	}
}

func TestNPCTalk_UnknownChoice(t *testing.T) {
	trees := map[string]*npc.Tree{
		"npc_wang_boss_001": {
			NpcID: "npc_wang_boss_001",
			Nodes: map[string]npc.Node{},
		},
	}
	r, _ := setupTestRouter(t, trees)
	body := bytes.NewBufferString(`{"npc_id":"npc_wang_boss_001","player_id":"p","choice_id":"unknown"}`)
	req := httptest.NewRequest("POST", "/v1/npc/talk", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "NPC_002" {
		t.Errorf("error = %v, want NPC_002", resp["error"])
	}
}

func TestNPCTalk_MissingFields(t *testing.T) {
	r, _ := setupTestRouter(t, map[string]*npc.Tree{})
	body := bytes.NewBufferString(`{"npc_id":"npc_x"}`) // missing player_id, choice_id
	req := httptest.NewRequest("POST", "/v1/npc/talk", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "NPC_002" {
		t.Errorf("error = %v, want NPC_002", resp["error"])
	}
}
