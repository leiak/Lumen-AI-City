package hub

import (
	"context"
	"testing"
	"time"
)

// waitFor 轮询等 cond 成立，超时 fail（比固定 sleep 稳）。
func waitFor(t *testing.T, timeout time.Duration, desc string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", desc)
}

func TestHub_RegisterBroadcastUnregister(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := New(nil)
	go h.Run(ctx)

	fc := newFakeConn()
	c := NewClient("c1", "player-1", fc, 4, nil)
	if err := h.Register(c); err != nil {
		t.Fatalf("Register: %v", err)
	}
	waitFor(t, time.Second, "connected=1", func() bool { return h.Stats().Connected == 1 })

	h.Broadcast([]byte(`{"type":"player_moved"}`))
	waitFor(t, time.Second, "delivered=1", func() bool { return h.Stats().Delivered == 1 })

	// 消息应落在 client 的 send 队列里（WritePump 未启动时不写 conn）
	select {
	case got := <-c.send:
		if string(got) != `{"type":"player_moved"}` {
			t.Errorf("send = %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("nothing in send queue")
	}

	h.Unregister(c)
	waitFor(t, time.Second, "connected=0", func() bool { return h.Stats().Connected == 0 })

	// 注销后不再投递
	before := h.Stats().Delivered
	h.Broadcast([]byte(`x`))
	waitFor(t, time.Second, "broadcast counted", func() bool { return h.Stats().Broadcast == 2 })
	if got := h.Stats().Delivered; got != before {
		t.Errorf("Delivered = %d, want unchanged %d", got, before)
	}
}

// 慢消费者：send 队列填满后再广播 → 驱逐 + Close(PolicyViolation)。
func TestHub_EvictsSlowConsumer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := New(nil)
	go h.Run(ctx)

	fc := newFakeConn()
	// 缓冲 1，且不启动 WritePump → 队列永不排空
	c := NewClient("slow", "player-slow", fc, 1, nil)
	if err := h.Register(c); err != nil {
		t.Fatalf("Register: %v", err)
	}
	waitFor(t, time.Second, "connected=1", func() bool { return h.Stats().Connected == 1 })

	h.Broadcast([]byte(`m1`)) // 填满缓冲
	waitFor(t, time.Second, "delivered=1", func() bool { return h.Stats().Delivered == 1 })

	h.Broadcast([]byte(`m2`)) // 塞不进 → 驱逐
	waitFor(t, time.Second, "evicted=1", func() bool { return h.Stats().Evicted == 1 })
	waitFor(t, time.Second, "connected=0", func() bool { return h.Stats().Connected == 0 })

	waitFor(t, time.Second, "conn closed", func() bool {
		_, closed, _, _, _ := fc.snapshot()
		return closed
	})
	_, _, code, reason, _ := fc.snapshot()
	if code != StatusPolicyViolation {
		t.Errorf("close code = %v, want %v", code, StatusPolicyViolation)
	}
	if reason != "slow consumer" {
		t.Errorf("close reason = %q, want %q", reason, "slow consumer")
	}
}

// 多客户端扇出：一条广播每个连接各收一份。
func TestHub_FanoutToAllClients(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := New(nil)
	go h.Run(ctx)

	clients := make([]*Client, 3)
	for i := range clients {
		clients[i] = NewClient("c", "p", newFakeConn(), 4, nil)
		if err := h.Register(clients[i]); err != nil {
			t.Fatalf("Register: %v", err)
		}
	}
	waitFor(t, time.Second, "connected=3", func() bool { return h.Stats().Connected == 3 })

	h.Broadcast([]byte(`fanout`))
	waitFor(t, time.Second, "delivered=3", func() bool { return h.Stats().Delivered == 3 })

	for i, c := range clients {
		select {
		case got := <-c.send:
			if string(got) != "fanout" {
				t.Errorf("client %d got %q", i, got)
			}
		case <-time.After(time.Second):
			t.Fatalf("client %d got nothing", i)
		}
	}
}

// ctx 取消 → 所有连接被 Close(GoingAway)，后续 Register 返回 ErrHubClosed。
func TestHub_ShutdownClosesClients(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	h := New(nil)
	go h.Run(ctx)

	fc := newFakeConn()
	c := NewClient("c1", "player-1", fc, 4, nil)
	if err := h.Register(c); err != nil {
		t.Fatalf("Register: %v", err)
	}
	waitFor(t, time.Second, "connected=1", func() bool { return h.Stats().Connected == 1 })

	cancel()

	waitFor(t, 2*time.Second, "conn closed", func() bool {
		_, closed, _, _, _ := fc.snapshot()
		return closed
	})
	_, _, code, _, _ := fc.snapshot()
	if code != StatusGoingAway {
		t.Errorf("close code = %v, want %v", code, StatusGoingAway)
	}

	if err := h.Register(NewClient("c2", "p2", newFakeConn(), 4, nil)); err != ErrHubClosed {
		t.Errorf("Register after shutdown = %v, want ErrHubClosed", err)
	}
	// Unregister / Broadcast 关停后必须静默返回，不能 panic 或阻塞
	h.Unregister(c)
	h.Broadcast([]byte(`ignored`))
}
