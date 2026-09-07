package hub

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestClient_WritePump_WritesQueuedMessages(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fc := newFakeConn()
	c := NewClient("c1", "p1", fc, 4, nil)
	go c.WritePump(ctx, 0) // 关心跳

	if !c.trySend([]byte(`m1`)) || !c.trySend([]byte(`m2`)) {
		t.Fatal("trySend failed on empty queue")
	}

	waitFor(t, time.Second, "2 writes", func() bool {
		w, _, _, _, _ := fc.snapshot()
		return len(w) == 2
	})
	w, _, _, _, _ := fc.snapshot()
	if string(w[0]) != "m1" || string(w[1]) != "m2" {
		t.Errorf("writes = %q, %q", w[0], w[1])
	}
}

// ctx 取消 → WritePump 返回（否则 shutdown 时 goroutine 泄漏）。
func TestClient_WritePump_ReturnsOnCtxCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	fc := newFakeConn()
	c := NewClient("c1", "p1", fc, 4, nil)

	done := make(chan struct{})
	go func() {
		c.WritePump(ctx, 0)
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("WritePump did not return after ctx cancel")
	}
}

// 写失败 → WritePump 返回，让 handler 走 Unregister 清理。
func TestClient_WritePump_ReturnsOnWriteError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fc := newFakeConn()
	fc.writeErr = errors.New("broken pipe")
	c := NewClient("c1", "p1", fc, 4, nil)

	done := make(chan struct{})
	go func() {
		c.WritePump(ctx, 0)
		close(done)
	}()

	c.trySend([]byte(`m1`))
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("WritePump did not return after write error")
	}
}

// pingInterval > 0 时定期 Ping。
func TestClient_WritePump_SendsPings(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fc := newFakeConn()
	c := NewClient("c1", "p1", fc, 4, nil)
	go c.WritePump(ctx, 10*time.Millisecond)

	waitFor(t, 2*time.Second, "at least 2 pings", func() bool {
		_, _, _, _, pings := fc.snapshot()
		return pings >= 2
	})
}

// send 队列满 → trySend 返回 false（hub 据此驱逐）。
func TestClient_TrySend_FullQueue(t *testing.T) {
	c := NewClient("c1", "p1", newFakeConn(), 2, nil)

	if !c.trySend([]byte(`m1`)) || !c.trySend([]byte(`m2`)) {
		t.Fatal("first 2 trySend should succeed")
	}
	if c.trySend([]byte(`m3`)) {
		t.Error("trySend on full queue = true, want false")
	}
}

// sendBuffer <= 0 时兜底成 16，不能造出零容量队列（那样第一条消息就被判慢消费者）。
func TestClient_ZeroSendBufferDefaults(t *testing.T) {
	c := NewClient("c1", "p1", newFakeConn(), 0, nil)
	if got := cap(c.send); got != 16 {
		t.Errorf("cap(send) = %d, want 16", got)
	}
}

// Close 幂等：重复调用只真正关一次（hub 驱逐 + handler defer 会各调一次）。
func TestClient_CloseIsIdempotent(t *testing.T) {
	fc := newFakeConn()
	c := NewClient("c1", "p1", fc, 4, nil)

	c.Close(StatusPolicyViolation, "slow consumer")
	c.Close(StatusNormalClosure, "bye")

	_, closed, code, reason, _ := fc.snapshot()
	if !closed {
		t.Fatal("conn not closed")
	}
	if code != StatusPolicyViolation || reason != "slow consumer" {
		t.Errorf("code/reason = %v/%q, want first call to win", code, reason)
	}
}

// ReadPump 在连接关闭后返回（不读就检测不到对端断开）。
func TestClient_ReadPump_ReturnsOnConnClose(t *testing.T) {
	fc := newFakeConn()
	c := NewClient("c1", "p1", fc, 4, nil)

	done := make(chan struct{})
	go func() {
		c.ReadPump(context.Background())
		close(done)
	}()

	c.Close(StatusNormalClosure, "bye")
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ReadPump did not return after conn close")
	}
}
