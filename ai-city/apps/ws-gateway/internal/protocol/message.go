// Package protocol 定义 ws-gateway → 浏览器的下行消息信封。
//
// Sprint 9 只有一种 type：player_moved。上行（浏览器 → server）暂无协议，
// 订阅即连接。
package protocol

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// 消息 type 常量
const (
	TypePlayerMoved = "player_moved"
	TypeNpcDialogue = "npc_dialogue"
)

// Envelope 是所有下行消息的外层结构。
//
//	{"type":"player_moved","trace_id":"uuid","ts_ms":1700000000000,"payload":{...}}
//
// payload 用 json.RawMessage：Redis 上收到的已经是 JSON 字节，直接透传省一次
// 反序列化 → 序列化的往返，也避免 float32 精度在 round-trip 中漂移。
type Envelope struct {
	Type    string          `json:"type"`
	TraceID string          `json:"trace_id"`
	TsMs    int64           `json:"ts_ms"`
	Payload json.RawMessage `json:"payload"`
}

// PlayerMoved 是 world-engine 序列化进 Redis 的消息体。
// 字段名必须与 apps/world-engine/src/world_grid.rs::PlayerPosition 的
// serde 输出一致（player_id 而非 entity_id —— entity_id 只存在于 gRPC proto）。
type PlayerMoved struct {
	PlayerID string  `json:"player_id"`
	TileID   string  `json:"tile_id"`
	X        float32 `json:"x"`
	Y        float32 `json:"y"`
	TsMs     int64   `json:"ts_ms"`
}

// NpcDialogue 是 agent-os（T01d）序列化进 Redis 的 NPC 对话消息体。
//
// 区分两种语义（前端按 reply_to_choice_id === null 区分）：
//   - active say：PlayerID="", TileID="", ReplyToChoiceID=nil
//     （NPC 主动说，所有附近玩家都收到，options 通常为 []）
//   - reply    ：PlayerID/TileID 非空，ReplyToChoiceID=&"<choice_id>"
//     （NPC 回复某个玩家的选项，options 至少 1 条）
//
// 字段名必须与 agent-os 输出对齐（snake_case）—— web 端按 JSON key 取值。
type NpcDialogue struct {
	NpcID           string         `json:"npc_id"`
	PlayerID        string         `json:"player_id"`         // "" when active say
	TileID          string         `json:"tile_id"`           // "" when active say
	Say             string         `json:"say"`
	Options         []DialogOption `json:"options"`           // 始终 []（非 null）—— 即使空也是 []DialogOption{}
	ReplyToChoiceID *string        `json:"reply_to_choice_id"` // nil → JSON null（active say）；非 nil → string（reply）
}

// DialogOption 是 NpcDialogue.Options 的元素。
// id 是稳定的机器可读 key（agent-os 内部路由用），text 是给玩家看的中文/本地化文本。
type DialogOption struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

// NewEnvelope 用原始 payload 字节包一个信封。
// raw 非法 JSON 时返回 error —— 宁可丢一条消息，也不要把坏 JSON 推给浏览器
// （web/src/lib/ws.ts 的 JSON.parse 会整条丢弃，且看不出是哪一层坏了）。
func NewEnvelope(msgType string, raw []byte) (*Envelope, error) {
	if !json.Valid(raw) {
		return nil, &InvalidPayloadError{Type: msgType}
	}
	return &Envelope{
		Type:    msgType,
		TraceID: uuid.NewString(),
		TsMs:    time.Now().UnixMilli(),
		Payload: json.RawMessage(raw),
	}, nil
}

// InvalidPayloadError payload 不是合法 JSON
type InvalidPayloadError struct {
	Type string
}

func (e *InvalidPayloadError) Error() string {
	return "invalid json payload for message type " + e.Type
}
