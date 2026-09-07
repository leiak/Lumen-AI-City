package hub

import (
	"context"
	"errors"
	"sync"

	"nhooyr.io/websocket"
)

// fakeConn 是 Conn 的测试替身：写入的消息进 writes，Close 记录码与原因。
// Read 阻塞到 ctx 取消（模拟没有上行流量的浏览器）。
type fakeConn struct {
	mu     sync.Mutex
	writes [][]byte
	pings  int
	closed bool
	code   websocket.StatusCode
	reason string

	// writeErr 非 nil 时 Write 直接失败（模拟坏连接）
	writeErr error
	// writeBlock 非 nil 时 Write 阻塞在它上（模拟 TCP 缓冲塞满）
	writeBlock chan struct{}
	// closedCh 在首次 Close 时关闭，供测试等待
	closedCh chan struct{}
}

func newFakeConn() *fakeConn {
	return &fakeConn{closedCh: make(chan struct{})}
}

func (f *fakeConn) Write(ctx context.Context, _ websocket.MessageType, p []byte) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	if f.writeBlock != nil {
		select {
		case <-f.writeBlock:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	// 拷一份：调用方复用 buffer 时不串数据
	cp := make([]byte, len(p))
	copy(cp, p)
	f.writes = append(f.writes, cp)
	return nil
}

func (f *fakeConn) Read(ctx context.Context) (websocket.MessageType, []byte, error) {
	select {
	case <-ctx.Done():
		return 0, nil, ctx.Err()
	case <-f.closedCh:
		return 0, nil, errors.New("closed")
	}
}

func (f *fakeConn) Ping(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pings++
	return nil
}

func (f *fakeConn) Close(code websocket.StatusCode, reason string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return errors.New("already closed")
	}
	f.closed = true
	f.code = code
	f.reason = reason
	close(f.closedCh)
	return nil
}

func (f *fakeConn) snapshot() (writes [][]byte, closed bool, code websocket.StatusCode, reason string, pings int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	writes = make([][]byte, len(f.writes))
	copy(writes, f.writes)
	return writes, f.closed, f.code, f.reason, f.pings
}
