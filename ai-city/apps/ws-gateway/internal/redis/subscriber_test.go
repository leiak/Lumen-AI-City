package redis

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/aicity/ws-gateway/internal/protocol"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// fakeBroadcaster 收集 Broadcast 调用。
type fakeBroadcaster struct {
	mu   sync.Mutex
	msgs [][]byte
	ch   chan struct{}
}

func newFakeBroadcaster() *fakeBroadcaster {
	return &fakeBroadcaster{ch: make(chan struct{}, 16)}
}

func (f *fakeBroadcaster) Broadcast(msg []byte) {
	f.mu.Lock()
	cp := make([]byte, len(msg))
	copy(cp, msg)
	f.msgs = append(f.msgs, cp)
	f.mu.Unlock()
	select {
	case f.ch <- struct{}{}:
	default:
	}
}

func (f *fakeBroadcaster) snapshot() [][]byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([][]byte, len(f.msgs))
	copy(out, f.msgs)
	return out
}

const rustPayload = `{"player_id":"p-1","tile_id":"tile_0_0","x":50.0,"y":50.0,"ts_ms":1700000000000}`

// handle 是纯函数，不需要 Redis —— 总是跑。
func TestHandle_WrapsPayloadInEnvelope(t *testing.T) {
	b := newFakeBroadcaster()
	handle(rustPayload, b, zap.NewNop())

	msgs := b.snapshot()
	if len(msgs) != 1 {
		t.Fatalf("Broadcast called %d times, want 1", len(msgs))
	}

	var env struct {
		Type    string               `json:"type"`
		TraceID string               `json:"trace_id"`
		TsMs    int64                `json:"ts_ms"`
		Payload protocol.PlayerMoved `json:"payload"`
	}
	if err := json.Unmarshal(msgs[0], &env); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if env.Type != protocol.TypePlayerMoved {
		t.Errorf("type = %q, want %q", env.Type, protocol.TypePlayerMoved)
	}
	if env.TraceID == "" || env.TsMs <= 0 {
		t.Errorf("trace_id/ts_ms = %q/%d", env.TraceID, env.TsMs)
	}
	if env.Payload.PlayerID != "p-1" || env.Payload.TileID != "tile_0_0" || env.Payload.X != 50 {
		t.Errorf("payload = %+v", env.Payload)
	}
}

// 坏 JSON 只丢这一条，不广播、不 panic。
func TestHandle_DropsInvalidJSON(t *testing.T) {
	b := newFakeBroadcaster()
	handle(`{"player_id":`, b, zap.NewNop())
	if got := len(b.snapshot()); got != 0 {
		t.Errorf("Broadcast called %d times, want 0", got)
	}
}

// 集成测：真 Redis 发一条 → 订阅者广播一条。
// WS_TEST_REDIS_URL 未设则跳过。
func TestPlayerMoved_Integration(t *testing.T) {
	url := os.Getenv("WS_TEST_REDIS_URL")
	if url == "" {
		t.Skip("WS_TEST_REDIS_URL not set; skipping redis integration test")
	}

	opt, err := goredis.ParseURL(url)
	if err != nil {
		t.Fatalf("ParseURL: %v", err)
	}
	rdb := goredis.NewClient(opt)
	defer rdb.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Fatalf("Ping: %v", err)
	}

	const channel = "aicity:test:ws:player:moved"
	b := newFakeBroadcaster()
	PlayerMoved(ctx, rdb, channel, b, zap.NewNop())

	// 订阅确认是异步的；重试 publish 直到收到第一条（避免竞态）
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := rdb.Publish(ctx, channel, rustPayload).Err(); err != nil {
			t.Fatalf("Publish: %v", err)
		}
		select {
		case <-b.ch:
			msgs := b.snapshot()
			var env struct {
				Type    string               `json:"type"`
				Payload protocol.PlayerMoved `json:"payload"`
			}
			if err := json.Unmarshal(msgs[0], &env); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if env.Type != protocol.TypePlayerMoved || env.Payload.PlayerID != "p-1" {
				t.Errorf("envelope = %+v", env)
			}
			return
		case <-time.After(200 * time.Millisecond):
		}
	}
	t.Fatal("no broadcast received within 5s")
}
