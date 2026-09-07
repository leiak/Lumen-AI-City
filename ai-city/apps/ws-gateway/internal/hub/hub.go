// Package hub 管理所有 WS 连接并做扇出广播。
//
// 经典 hub pattern：**单个 goroutine 独占 clients map**，外部只通过
// register / unregister / broadcast 三个 channel 交互 —— 于是 map 无需锁。
//
// Sprint 9 MVP 不做 tile/region 过滤，一条 player_moved 广播给所有连接
// （量级：9 tile × 个位数玩家）。过滤留到 Sprint 10+。
package hub

import (
	"context"
	"errors"
	"sync/atomic"

	"go.uber.org/zap"
)

// ErrHubClosed hub 已停止（ctx 取消后），不再接受注册 / 广播
var ErrHubClosed = errors.New("hub closed")

// Stats 是可观测计数器快照
type Stats struct {
	Connected int64  `json:"connected"`
	Delivered uint64 `json:"delivered"`
	Broadcast uint64 `json:"broadcast"`
	Evicted   uint64 `json:"evicted"`
}

type Hub struct {
	register   chan *Client
	unregister chan *Client
	broadcast  chan []byte
	done       chan struct{}

	// 仅 Run goroutine 访问，无需锁
	clients map[*Client]struct{}

	logger *zap.Logger

	connected atomic.Int64
	delivered atomic.Uint64
	broadcasts atomic.Uint64
	evicted   atomic.Uint64
}

func New(logger *zap.Logger) *Hub {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Hub{
		// register/unregister 留点缓冲，避免连接风暴时握手 goroutine 排队
		register:   make(chan *Client, 32),
		unregister: make(chan *Client, 32),
		broadcast:  make(chan []byte, 256),
		done:       make(chan struct{}),
		clients:    make(map[*Client]struct{}),
		logger:     logger,
	}
}

// Run 阻塞运行事件循环，直到 ctx 取消。调用方通常 `go hub.Run(appCtx)`。
func (h *Hub) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			// done 必须在断开连接**之前**关：否则 Register/Broadcast 在
			// "已开始关停但 done 未关" 的窗口里仍会成功，塞进无人消费的 chan。
			close(h.done)

			// 断开所有连接。close 走独立 goroutine，避免网络 I/O 拖住退出。
			for c := range h.clients {
				delete(h.clients, c)
				h.connected.Add(-1)
				go c.close(StatusGoingAway, "server shutting down")
			}
			h.logger.Info("hub stopped")
			return

		case c := <-h.register:
			h.clients[c] = struct{}{}
			h.connected.Add(1)
			h.logger.Info("client registered",
				zap.String("client_id", c.ID),
				zap.String("player_id", c.PlayerID),
				zap.Int("total", len(h.clients)))

		case c := <-h.unregister:
			if _, ok := h.clients[c]; ok {
				delete(h.clients, c)
				h.connected.Add(-1)
				h.logger.Info("client unregistered",
					zap.String("client_id", c.ID),
					zap.Int("total", len(h.clients)))
			}

		case msg := <-h.broadcast:
			h.broadcasts.Add(1)
			for c := range h.clients {
				if c.trySend(msg) {
					h.delivered.Add(1)
					continue
				}
				// 写队列满 = 慢消费者。踢掉，绝不让它拖住 hub。
				delete(h.clients, c)
				h.connected.Add(-1)
				h.evicted.Add(1)
				h.logger.Warn("evicting slow consumer",
					zap.String("client_id", c.ID),
					zap.String("player_id", c.PlayerID))
				go c.close(StatusPolicyViolation, "slow consumer")
			}
		}
	}
}

// Register 登记一个新连接；hub 已停则返回 ErrHubClosed。
//
// done 必须**先单独判一次**：register 有缓冲，若和 done 放在同一个 select 里，
// 两个 case 同时 ready 时 Go 随机挑一个 —— 于是关停后仍可能"注册成功"，
// 而事件循环已退出，这个连接就成了永不注册也永不关闭的孤儿。
func (h *Hub) Register(c *Client) error {
	select {
	case <-h.done:
		return ErrHubClosed
	default:
	}
	select {
	case h.register <- c:
		return nil
	case <-h.done:
		return ErrHubClosed
	}
}

// Unregister 注销连接；hub 已停时静默返回（连接自己也在退出路上）。
func (h *Hub) Unregister(c *Client) {
	select {
	case <-h.done:
		return
	default:
	}
	select {
	case h.unregister <- c:
	case <-h.done:
	}
}

// Broadcast 把一条已序列化的消息扇出给所有连接。
//
// 非阻塞：broadcast chan 满（hub 落后于 Redis 消息速率）时丢弃这一条并告警，
// 而不是反压到 Redis 订阅循环上 —— 位置事件是可丢的最新态快照。
func (h *Hub) Broadcast(msg []byte) {
	select {
	case <-h.done:
		return
	default:
	}
	select {
	case h.broadcast <- msg:
	default:
		h.logger.Warn("broadcast queue full, dropping message")
	}
}

func (h *Hub) Stats() Stats {
	return Stats{
		Connected: h.connected.Load(),
		Delivered: h.delivered.Load(),
		Broadcast: h.broadcasts.Load(),
		Evicted:   h.evicted.Load(),
	}
}
