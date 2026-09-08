package protocol

import (
	"bytes"
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

// agent-os（T01d）输出 active say 的实际形状：
//
//	{"npc_id":"npc_wang_boss_001","player_id":"","tile_id":"","say":"来了您嘞！","options":[],"reply_to_choice_id":null}
//
// 注意：player_id/tile_id 是空串而非 null（前端用空串哨兵），options 是 [] 而非 null。
const npcDialogueActiveSay = `{"npc_id":"npc_wang_boss_001","player_id":"","tile_id":"","say":"来了您嘞！","options":[],"reply_to_choice_id":null}`

// agent-os（T01d）输出 reply 的实际形状 —— 含 reply_to_choice_id 和至少一个 option。
const npcDialogueReply = `{"npc_id":"npc_wang_boss_001","player_id":"player-1","tile_id":"tile_0_0","say":"回您一句。","options":[{"id":"ok","text":"好的"}],"reply_to_choice_id":"ask_business"}`

func TestNpcDialogue_DecodesAgentOSActiveSay(t *testing.T) {
	var d NpcDialogue
	if err := json.Unmarshal([]byte(npcDialogueActiveSay), &d); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if d.NpcID != "npc_wang_boss_001" {
		t.Errorf("NpcID = %q", d.NpcID)
	}
	if d.PlayerID != "" || d.TileID != "" {
		t.Errorf("active say 应有 player_id=\"\" tile_id=\"\"，got (%q,%q)", d.PlayerID, d.TileID)
	}
	if d.Say != "来了您嘞！" {
		t.Errorf("Say = %q", d.Say)
	}
	if len(d.Options) != 0 {
		t.Errorf("active say 应有 options=[]，got %v", d.Options)
	}
	if d.ReplyToChoiceID != nil {
		t.Errorf("active say 应有 reply_to_choice_id=nil，got %v", *d.ReplyToChoiceID)
	}
}

func TestNpcDialogue_DecodesAgentOSReply(t *testing.T) {
	var d NpcDialogue
	if err := json.Unmarshal([]byte(npcDialogueReply), &d); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if d.NpcID != "npc_wang_boss_001" || d.PlayerID != "player-1" || d.TileID != "tile_0_0" {
		t.Errorf("ids mismatch: %+v", d)
	}
	if d.Say != "回您一句。" {
		t.Errorf("Say = %q", d.Say)
	}
	if len(d.Options) != 1 || d.Options[0].ID != "ok" || d.Options[0].Text != "好的" {
		t.Errorf("options mismatch: %+v", d.Options)
	}
	if d.ReplyToChoiceID == nil || *d.ReplyToChoiceID != "ask_business" {
		t.Errorf("reply_to_choice_id = %v, want \"ask_business\"", d.ReplyToChoiceID)
	}
}

// 字段名必须与 agent-os 输出对齐（snake_case），且不含 agent_id 这种别名。
func TestNpcDialogue_JSONTagsAreSnakeCase(t *testing.T) {
	b, err := json.Marshal(NpcDialogue{
		NpcID:           "npc_x",
		PlayerID:        "p1",
		TileID:          "tile_0_0",
		Say:             "hi",
		Options:         []DialogOption{{ID: "yes", Text: "好"}},
		ReplyToChoiceID: nil,
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	for _, k := range []string{"npc_id", "player_id", "tile_id", "say", "options", "reply_to_choice_id"} {
		if _, ok := m[k]; !ok {
			t.Errorf("missing key %q in %s", k, b)
		}
	}
	for _, bad := range []string{"agent_id", "npcId", "replyToChoiceId"} {
		if _, ok := m[bad]; ok {
			t.Errorf("payload 不应含 camelCase/别名 %q（got %s）", bad, b)
		}
	}
}

// 关键不变量：空 options 必须序列化为 []，不是 null。
// 后端 (a2a-gateway / agent-os) 写到 Redis 时若序列化成 null 会让 web client 的
// for-of 循环炸 —— Go 默认会把 nil slice 序列化成 null。
//
//	[]DialogOption{} → "options":[]
//	nil               → "options":null   ← 我们要避免这个
func TestNpcDialogue_OptionsEmptyArrayNotNull(t *testing.T) {
	b, err := json.Marshal(NpcDialogue{
		NpcID:   "npc_x",
		Say:     "hi",
		Options: []DialogOption{}, // 显式空切片（非 nil）
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !bytes.Contains(b, []byte(`"options":[]`)) {
		t.Errorf("expected \"options\":[] not null; got %s", b)
	}
	if bytes.Contains(b, []byte(`"options":null`)) {
		t.Errorf("options 序列化为 null 了（应该是 []）; got %s", b)
	}
}

// 关键不变量：reply_to_choice_id 在 active say 时必须序列化为 null，
// 方便前端用 d.reply_to_choice_id === null 区分 active say vs reply。
//
//	*string nil  → "reply_to_choice_id":null
//	*string &""  → "reply_to_choice_id":""   ← 罕见但合法（显式空选择 id）
func TestNpcDialogue_ReplyToChoiceIDNullable(t *testing.T) {
	b, err := json.Marshal(NpcDialogue{
		NpcID:           "npc_x",
		Say:             "hi",
		ReplyToChoiceID: nil,
	})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !bytes.Contains(b, []byte(`"reply_to_choice_id":null`)) {
		t.Errorf("expected \"reply_to_choice_id\":null; got %s", b)
	}
}

// 信封层 round-trip —— Redis 上收到 raw JSON 后被 NewEnvelope 包成 envelope，
// ws 客户端应能解析回 NpcDialogue 类型（payload 透传）。
func TestNewEnvelope_NpcDialogue_RoundTrip(t *testing.T) {
	env, err := NewEnvelope(TypeNpcDialogue, []byte(npcDialogueActiveSay))
	if err != nil {
		t.Fatalf("NewEnvelope: %v", err)
	}
	if env.Type != TypeNpcDialogue {
		t.Errorf("Type = %q, want %q", env.Type, TypeNpcDialogue)
	}

	wire, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("Marshal envelope: %v", err)
	}

	// 浏览器侧看到的形状
	var back struct {
		Type    string      `json:"type"`
		TraceID string      `json:"trace_id"`
		TsMs    int64       `json:"ts_ms"`
		Payload NpcDialogue `json:"payload"`
	}
	if err := json.Unmarshal(wire, &back); err != nil {
		t.Fatalf("Unmarshal envelope: %v", err)
	}
	if back.Type != TypeNpcDialogue {
		t.Errorf("type round-trip = %q", back.Type)
	}
	if back.Payload.NpcID != "npc_wang_boss_001" || back.Payload.Say != "来了您嘞！" {
		t.Errorf("payload round-trip mismatch: %+v", back.Payload)
	}
}
