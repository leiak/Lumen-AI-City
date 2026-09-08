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

func (f *fakeBroadcaster) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.msgs)
}

const rustPayload = `{"player_id":"p-1","tile_id":"tile_0_0","x":50.0,"y":50.0,"ts_ms":1700000000000}`

// npcPayload 是 agent-os 发的 raw NpcDialogue JSON —— 注意 reply_to_choice_id
// 是 null（active say：所有附近玩家都收到，options 为 []）。
const npcPayload = `{"npc_id":"npc-1","player_id":"","tile_id":"","say":"hello","options":[],"reply_to_choice_id":null}`

// broadcastFilter 是 PlayerMoved 内部的 Filter —— marshal 信封 → 广播。
// 纯函数，不需要 Redis —— 总是跑。
func TestBroadcastFilter_WrapsAndBroadcasts(t *testing.T) {
	b := newFakeBroadcaster()
	filter := broadcastFilter(b, zap.NewNop())

	env, err := protocol.NewEnvelope(protocol.TypePlayerMoved, []byte(rustPayload))
	if err != nil {
		t.Fatalf("NewEnvelope: %v", err)
	}
	if consumed := filter(*env); consumed {
		t.Errorf("Filter returned true (drop), want false (pass-through)")
	}

	msgs := b.snapshot()
	if len(msgs) != 1 {
		t.Fatalf("Broadcast called %d times, want 1", len(msgs))
	}

	var back struct {
		Type    string               `json:"type"`
		TraceID string               `json:"trace_id"`
		TsMs    int64                `json:"ts_ms"`
		Payload protocol.PlayerMoved `json:"payload"`
	}
	if err := json.Unmarshal(msgs[0], &back); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if back.Type != protocol.TypePlayerMoved {
		t.Errorf("type = %q, want %q", back.Type, protocol.TypePlayerMoved)
	}
	if back.TraceID == "" || back.TsMs <= 0 {
		t.Errorf("trace_id/ts_ms = %q/%d", back.TraceID, back.TsMs)
	}
	if back.Payload.PlayerID != "p-1" || back.Payload.TileID != "tile_0_0" || back.Payload.X != 50 {
		t.Errorf("payload = %+v", back.Payload)
	}
}

// 坏 JSON：NewEnvelope 失败 → Filter 不被调 → 广播 0 次。
// 纯函数，不需要 Redis —— 总是跑。
func TestHandleChannelMessage_DropsInvalidJSON(t *testing.T) {
	b := newFakeBroadcaster()
	cfg := ChannelConfig{
		Channel: "test:invalid",
		Type:    protocol.TypePlayerMoved,
		Filter:  broadcastFilter(b, zap.NewNop()),
	}
	handleChannelMessage(cfg, `{"player_id":`, zap.NewNop())
	if got := b.count(); got != 0 {
		t.Errorf("Broadcast called %d times, want 0", got)
	}
}

// RunMultiSubscriber 空 configs 立即返 nil（不连 Redis）。
// 不需要 Redis —— 总是跑。
func TestRunMultiSubscriber_EmptyConfigsReturnsImmediately(t *testing.T) {
	rdb := goredis.NewClient(&goredis.Options{Addr: "127.0.0.1:1"})
	defer rdb.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- RunMultiSubscriber(ctx, rdb, nil, zap.NewNop()) }()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("RunMultiSubscriber(nil configs) returned %v, want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("RunMultiSubscriber(nil configs) did not return within 1s")
	}
}

// ctx 取消时所有 channel goroutine 应立即退出。
// 不需要 Redis（用不可达地址让 runChannelOnce 持续重连，验证 cancel 能打破循环）。
// 总是跑。
func TestRunMultiSubscriber_StopsOnContextCancel(t *testing.T) {
	rdb := goredis.NewClient(&goredis.Options{Addr: "127.0.0.1:1"})
	defer rdb.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	configs := []ChannelConfig{
		{Channel: "test:fake:1", Type: protocol.TypePlayerMoved,
			Filter: func(protocol.Envelope) bool { return false }},
		{Channel: "test:fake:2", Type: protocol.TypeNpcDialogue,
			Filter: func(protocol.Envelope) bool { return false }},
	}
	go func() { done <- RunMultiSubscriber(ctx, rdb, configs, zap.NewNop()) }()

	// 给点时间让 goroutine 起来订阅（会失败但循环在重试）
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// good
	case <-time.After(3 * time.Second):
		t.Fatal("RunMultiSubscriber did not return within 3s of ctx cancel")
	}
}

// 集成测：真 Redis 发一条 → PlayerMoved 广播一条（Sprint 9 兼容性）。
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

// 集成测：多频道并发订阅 → 各自 Filter 被正确触发。
// WS_TEST_REDIS_URL 未设则跳过。
func TestMultiChannelSubscriber_DispatchesBothTypes(t *testing.T) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Fatalf("Ping: %v", err)
	}

	stamp := time.Now().Format("150405.000000")
	ch1 := "test:ws:player:moved:" + stamp
	ch2 := "test:ws:npc:dialogue:" + stamp
	defer rdb.Del(context.Background(), ch1, ch2)

	type result struct {
		channel string
		envType string
	}
	out := make(chan result, 16)

	done := make(chan error, 1)
	configs := []ChannelConfig{
		{Channel: ch1, Type: protocol.TypePlayerMoved, Filter: func(env protocol.Envelope) bool {
			out <- result{ch1, env.Type}
			return false
		}},
		{Channel: ch2, Type: protocol.TypeNpcDialogue, Filter: func(env protocol.Envelope) bool {
			out <- result{ch2, env.Type}
			return false
		}},
	}
	go func() { done <- RunMultiSubscriber(ctx, rdb, configs, zap.NewNop()) }()

	publishWithRetry := func(channel, payload string) {
		d := time.Now().Add(3 * time.Second)
		for time.Now().Before(d) {
			if err := rdb.Publish(ctx, channel, payload).Err(); err == nil {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatalf("Publish to %s failed repeatedly", channel)
	}

	publishWithRetry(ch1, rustPayload)
	publishWithRetry(ch2, npcPayload)

	want := map[string]string{
		ch1: protocol.TypePlayerMoved,
		ch2: protocol.TypeNpcDialogue,
	}
	published := map[string]bool{}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && len(published) < 2 {
		select {
		case r := <-out:
			if want[r.channel] == "" {
				t.Errorf("unexpected channel %s", r.channel)
				continue
			}
			if r.envType != want[r.channel] {
				t.Errorf("channel=%s type=%s, want %s", r.channel, r.envType, want[r.channel])
			}
			published[r.channel] = true
		case <-time.After(200 * time.Millisecond):
		}
	}

	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("RunMultiSubscriber did not exit within 3s of cancel")
	}

	if len(published) != 2 {
		t.Fatalf("only received from %d/2 channels: %+v", len(published), published)
	}
}