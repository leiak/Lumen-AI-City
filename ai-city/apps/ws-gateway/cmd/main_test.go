package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aicity/ws-gateway/internal/hub"
	"github.com/aicity/ws-gateway/internal/protocol"
	"go.uber.org/zap"
	"nhooyr.io/websocket"
)

// fakeConn 是 hub.Conn 的替身：Write 进入 writes，供契约测试断言"到达哪条连接"。
type fakeConn struct {
	mu     sync.Mutex
	writes [][]byte
}

func newFakeConn() *fakeConn { return &fakeConn{} }

func (f *fakeConn) Write(_ context.Context, _ websocket.MessageType, p []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := make([]byte, len(p))
	copy(cp, p)
	f.writes = append(f.writes, cp)
	return nil
}

func (f *fakeConn) Read(ctx context.Context) (websocket.MessageType, []byte, error) {
	<-ctx.Done()
	return 0, nil, errors.New("closed")
}

func (f *fakeConn) Ping(context.Context) error               { return nil }
func (f *fakeConn) Close(websocket.StatusCode, string) error { return nil }

func (f *fakeConn) msgs() [][]byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.writes
}

func waitConn(t *testing.T, d time.Duration, desc string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", desc)
}

// Sprint 13：npc_dialogue 载荷里带 player_id 时才走 SendToPlayer（否则广播）。
func TestNpcDialogueTarget(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		want    string
	}{
		{"npc reply to player", `{"npc_id":"npc_wang_boss_001","player_id":"p3","say":"好嘞","options":[],"reply_to_choice_id":"ask_food"}`, "p3"},
		{"active say (broadcast)", `{"npc_id":"npc_wang_boss_001","player_id":"","say":"来了您嘞！","options":[],"reply_to_choice_id":null}`, ""},
		{"missing player key", `{"npc_id":"n","say":"x"}`, ""},
	}
	for _, c := range cases {
		env, err := protocol.NewEnvelope(protocol.TypeNpcDialogue, []byte(c.payload))
		if err != nil {
			t.Fatalf("%s: NewEnvelope: %v", c.name, err)
		}
		if got := npcDialogueTarget(env.Payload); got != c.want {
			t.Errorf("%s: npcDialogueTarget = %q, want %q", c.name, got, c.want)
		}
	}

	// 坏 JSON 在订阅层就被 NewEnvelope 拦下，这里直接测 helper 的防御分支。
	if got := npcDialogueTarget(json.RawMessage(`not json`)); got != "" {
		t.Errorf("bad json: npcDialogueTarget = %q, want empty", got)
	}
}

// agent-os → ws-gateway 契约：mock（fake conn + 真 hub）断言专属台词只到达目标连接，
// 主动广播 / player_moved 则到达所有连接。payload 即 agent-os ActionDispatcher.say 的输出。
func TestAgentOsNpcDialogueReachesTargetConnection(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := hub.New(nil)
	go h.Run(ctx)

	type css struct{ fc *fakeConn }
	mk := func(id, pid string) *css {
		fc := newFakeConn()
		c := hub.NewClient(id, pid, fc, 8, nil)
		if err := h.Register(c); err != nil {
			t.Fatalf("register %s: %v", id, err)
		}
		go c.WritePump(ctx, 0) // 写 pump 把 send 队列落到 fake conn，模拟真实连接
		return &css{fc: fc}
	}
	target := mk("a", "player-A")
	other := mk("b", "player-B")
	anon := mk("anon", "")
	waitConn(t, time.Second, "connected=3", func() bool { return h.Stats().Connected == 3 })

	logger := zap.NewNop()

	// 1) welcome（专属，player_id 非空）→ 只到 player-A 连接
	envW, err := protocol.NewEnvelope(protocol.TypeNpcDialogue, []byte(
		`{"npc_id":"npc_wang_boss_001","player_id":"player-A","tile_id":"tile_0_0","say":"哟，您可算回来了！","options":[{"id":"ask_food","text":"有什么菜？"}],"reply_to_choice_id":null}`))
	if err != nil {
		t.Fatalf("welcome envelope: %v", err)
	}
	publishEnvelope(h, logger, *envW)
	waitConn(t, time.Second, "target got welcome", func() bool { return len(target.fc.msgs()) == 1 })
	if got := string(target.fc.msgs()[0]); !strings.Contains(got, "哟，您可算回来了！") {
		t.Errorf("target conn missing welcome, got: %s", got)
	}
	if n := len(other.fc.msgs()); n != 0 {
		t.Errorf("player-B received %d msgs, want 0", n)
	}
	if n := len(anon.fc.msgs()); n != 0 {
		t.Errorf("anonymous received %d msgs, want 0", n)
	}

	// 2) 主动广播（player_id 空）→ 所有连接都收到
	envA, err := protocol.NewEnvelope(protocol.TypeNpcDialogue, []byte(
		`{"npc_id":"npc_wang_boss_001","player_id":"","tile_id":"","say":"欢迎光临小店。","options":[],"reply_to_choice_id":null}`))
	if err != nil {
		t.Fatalf("active envelope: %v", err)
	}
	publishEnvelope(h, logger, *envA)
	waitConn(t, time.Second, "active say reaches all", func() bool {
		return len(target.fc.msgs()) == 2 && len(other.fc.msgs()) == 1 && len(anon.fc.msgs()) == 1
	})

	// 3) player_moved → 广播给所有
	envM, err := protocol.NewEnvelope(protocol.TypePlayerMoved, []byte(
		`{"player_id":"player-A","tile_id":"tile_0_0","x":50,"y":50,"ts_ms":1}`))
	if err != nil {
		t.Fatalf("moved envelope: %v", err)
	}
	publishEnvelope(h, logger, *envM)
	waitConn(t, time.Second, "player_moved reaches all", func() bool {
		return len(target.fc.msgs()) == 3 && len(other.fc.msgs()) == 2 && len(anon.fc.msgs()) == 2
	})
}

// Sprint 13：npc_moved 是全地图可见的移动事件 → Broadcast 给所有连接，
// 不触发 SendToPlayer（payload 是 npc_id，不是 player_id）。
func TestNpcMovedBroadcast(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := hub.New(nil)
	go h.Run(ctx)

	mk := func(id string) *fakeConn {
		fc := newFakeConn()
		c := hub.NewClient(id, "", fc, 8, nil)
		if err := h.Register(c); err != nil {
			t.Fatalf("register %s: %v", id, err)
		}
		go c.WritePump(ctx, 0)
		return fc
	}
	c1 := mk("a")
	c2 := mk("b")
	c3 := mk("c")
	waitConn(t, time.Second, "connected=3", func() bool { return h.Stats().Connected == 3 })

	logger := zap.NewNop()
	env, err := protocol.NewEnvelope(protocol.TypeNpcMoved, []byte(
		`{"npc_id":"npc_wang_boss_001","tile_id":"tile_1_0","x":150,"y":50,"ts_ms":1}`))
	if err != nil {
		t.Fatalf("npc_moved envelope: %v", err)
	}
	publishEnvelope(h, logger, *env)
	waitConn(t, time.Second, "npc_moved reaches all", func() bool {
		return len(c1.msgs()) == 1 && len(c2.msgs()) == 1 && len(c3.msgs()) == 1
	})
	if got := string(c1.msgs()[0]); !strings.Contains(got, `"type":"npc_moved"`) {
		t.Errorf("npc_moved envelope type missing, got: %s", got)
	}
}
