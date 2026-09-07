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
