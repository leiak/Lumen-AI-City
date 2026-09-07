package hub

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
	"nhooyr.io/websocket"
)

// 关闭码别名：让 hub / 测试不必直接 import websocket
type StatusCode = websocket.StatusCode

const (
	StatusNormalClosure  = websocket.StatusNormalClosure
	StatusGoingAway      = websocket.StatusGoingAway
	StatusPolicyViolation = websocket.StatusPolicyViolation
	StatusInternalError  = websocket.StatusInternalError
)

// writeTimeout 单条消息的写超时。超时即视为连接坏死并断开
// （TCP 缓冲塞满 + 对端不读时，Write 会永久阻塞）。
const writeTimeout = 5 * time.Second

// Conn 是 *websocket.Conn 用到的方法子集，抽出来是为了单测能塞假连接。
type Conn interface {
	Write(ctx context.Context, typ websocket.MessageType, p []byte) error
	Read(ctx context.Context) (websocket.MessageType, []byte, error)
	Ping(ctx context.Context) error
	Close(code websocket.StatusCode, reason string) error
}

// Client 是一条 WS 连接。
//
// send 是有界队列（config.SendBuffer，默认 16）：hub 写不进就判定慢消费者并
// 驱逐，绝不阻塞 hub 的事件循环。
type Client struct {
	ID       string
	PlayerID string

	conn   Conn
	send   chan []byte
	logger *zap.Logger

	closeOnce sync.Once
}

func NewClient(id, playerID string, conn Conn, sendBuffer int, logger *zap.Logger) *Client {
	if sendBuffer <= 0 {
		sendBuffer = 16
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Client{
		ID:       id,
		PlayerID: playerID,
		conn:     conn,
		send:     make(chan []byte, sendBuffer),
		logger:   logger,
	}
}

// trySend 非阻塞入队；队列满返回 false（调用方 hub 会驱逐该客户端）。
func (c *Client) trySend(msg []byte) bool {
	select {
	case c.send <- msg:
		return true
	default:
		return false
	}
}

// close 幂等关闭底层连接。
func (c *Client) close(code StatusCode, reason string) {
	c.closeOnce.Do(func() {
		if err := c.conn.Close(code, reason); err != nil {
			c.logger.Debug("ws close returned error",
				zap.String("client_id", c.ID), zap.Error(err))
		}
	})
}

// Close 供外部（handler defer）调用的幂等关闭。
func (c *Client) Close(code StatusCode, reason string) {
	c.close(code, reason)
}

// WritePump 把 send 队列写到连接上，并按 pingInterval 发心跳。
// 返回即表示该连接已不可用，调用方须 Unregister + Close。
//
// pingInterval <= 0 时关闭心跳。
func (c *Client) WritePump(ctx context.Context, pingInterval time.Duration) {
	var pingC <-chan time.Time
	if pingInterval > 0 {
		t := time.NewTicker(pingInterval)
		defer t.Stop()
		pingC = t.C
	}

	for {
		select {
		case <-ctx.Done():
			return

		case msg := <-c.send:
			wctx, cancel := context.WithTimeout(ctx, writeTimeout)
			err := c.conn.Write(wctx, websocket.MessageText, msg)
			cancel()
			if err != nil {
				c.logger.Debug("ws write failed",
					zap.String("client_id", c.ID), zap.Error(err))
				return
			}

		case <-pingC:
			pctx, cancel := context.WithTimeout(ctx, writeTimeout)
			err := c.conn.Ping(pctx)
			cancel()
			if err != nil {
				c.logger.Debug("ws ping failed",
					zap.String("client_id", c.ID), zap.Error(err))
				return
			}
		}
	}
}

// ReadPump 读并丢弃上行消息，直到出错 / ctx 取消。
//
// Sprint 9 没有上行协议，但仍必须读：nhooyr 的 Read 是处理 close/pong 帧的
// 唯一入口，不读就检测不到对端断开（连接会一直挂在 hub 里）。
func (c *Client) ReadPump(ctx context.Context) {
	for {
		if _, _, err := c.conn.Read(ctx); err != nil {
			return
		}
	}
}
