package protocol

import (
	"encoding/json"
	"testing"
)

// rustPayload 是 world-engine `serde_json::to_string(&PlayerPosition)` 的实际输出形状
// （apps/world-engine/src/world_grid.rs:25 + grpc.rs:103）。
const rustPayload = `{"player_id":"1fce8ddd-0000-0000-0000-000000000001","tile_id":"tile_0_0","x":50.0,"y":50.0,"ts_ms":1700000000000}`

func TestPlayerMoved_DecodesRustSerdeOutput(t *testing.T) {
	var p PlayerMoved
	if err := json.Unmarshal([]byte(rustPayload), &p); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if p.PlayerID != "1fce8ddd-0000-0000-0000-000000000001" {
		t.Errorf("PlayerID = %q", p.PlayerID)
	}
	if p.TileID != "tile_0_0" {
		t.Errorf("TileID = %q, want tile_0_0", p.TileID)
	}
	if p.X != 50 || p.Y != 50 {
		t.Errorf("(x,y) = (%v,%v), want (50,50)", p.X, p.Y)
	}
	if p.TsMs != 1700000000000 {
		t.Errorf("TsMs = %d", p.TsMs)
	}
}

// 字段名必须是 player_id 而不是 entity_id —— entity_id 只存在于 gRPC proto，
// Redis 上流的是 Rust struct 的 serde 输出。
func TestPlayerMoved_JSONTagsAreSnakeCase(t *testing.T) {
	b, err := json.Marshal(PlayerMoved{
		PlayerID: "p1", TileID: "tile_1_0", X: 150, Y: 50, TsMs: 1,
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	for _, k := range []string{"player_id", "tile_id", "x", "y", "ts_ms"} {
		if _, ok := m[k]; !ok {
			t.Errorf("missing key %q in %s", k, b)
		}
	}
	if _, ok := m["entity_id"]; ok {
		t.Error("payload 不应含 entity_id（那是 gRPC proto 内部字段名）")
	}
}

func TestNewEnvelope_RoundTrip(t *testing.T) {
	env, err := NewEnvelope(TypePlayerMoved, []byte(rustPayload))
	if err != nil {
		t.Fatalf("NewEnvelope: %v", err)
	}
	if env.Type != TypePlayerMoved {
		t.Errorf("Type = %q, want %q", env.Type, TypePlayerMoved)
	}
	if env.TraceID == "" {
		t.Error("TraceID empty")
	}
	if env.TsMs <= 0 {
		t.Errorf("TsMs = %d, want > 0", env.TsMs)
	}

	wire, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("Marshal envelope: %v", err)
	}

	// 浏览器侧看到的形状：{type, trace_id, ts_ms, payload{...}}
	var back struct {
		Type    string      `json:"type"`
		TraceID string      `json:"trace_id"`
		TsMs    int64       `json:"ts_ms"`
		Payload PlayerMoved `json:"payload"`
	}
	if err := json.Unmarshal(wire, &back); err != nil {
		t.Fatalf("Unmarshal envelope: %v", err)
	}
	if back.Type != TypePlayerMoved || back.TraceID != env.TraceID || back.TsMs != env.TsMs {
		t.Errorf("envelope round-trip mismatch: %+v", back)
	}
	if back.Payload.PlayerID != "1fce8ddd-0000-0000-0000-000000000001" || back.Payload.X != 50 {
		t.Errorf("payload round-trip mismatch: %+v", back.Payload)
	}
}

// payload 必须原样透传（不重新序列化），否则 float32 精度会漂移。
func TestNewEnvelope_PayloadPassthrough(t *testing.T) {
	raw := `{"player_id":"p1","x":33.333332,"y":0.1,"tile_id":"tile_0_0","ts_ms":1}`
	env, err := NewEnvelope(TypePlayerMoved, []byte(raw))
	if err != nil {
		t.Fatalf("NewEnvelope: %v", err)
	}
	if string(env.Payload) != raw {
		t.Errorf("Payload = %s, want verbatim %s", env.Payload, raw)
	}
}

func TestNewEnvelope_RejectsInvalidJSON(t *testing.T) {
	for _, bad := range []string{``, `{`, `not json`, `{"a":}`} {
		if _, err := NewEnvelope(TypePlayerMoved, []byte(bad)); err == nil {
			t.Errorf("NewEnvelope(%q) err = nil, want error", bad)
		}
	}
}
