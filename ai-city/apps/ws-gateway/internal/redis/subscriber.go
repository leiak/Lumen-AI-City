// Package redis 订阅 world-engine 与 agent-os 的事件频道，包信封后交给 hub 扇出。
//
// 与 api-gateway/internal/subscriber/player_moved.go 是**两个独立订阅者**，
// 消费同一频道：api-gateway 写 PG player_position，这里推 WS。
// Redis pub/sub 天然支持多订阅者，两边互不影响。
//
// Sprint 11+（T02b）：支持多频道（player_moved + npc_dialogue）。每个频道独立
// goroutine、独立重连循环、独立指数 backoff（沿用 Sprint 9 单频道模式）。
package redis

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/aicity/ws-gateway/internal/protocol"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Broadcaster 是 hub.Hub 的最小接口（便于单测注入假实现）。
type Broadcaster interface {
	Broadcast(msg []byte)
}

// ChannelConfig 把一个 Redis 频道绑定到一个类型化的 Filter 回调。
//
// Type 是包信封用的 type 标签（e.g. protocol.TypePlayerMoved）。
// Redis 上的 payload 假定为**内部 payload 对象**（raw PlayerMoved / raw
// NpcDialogue），不是完整 Envelope —— 订阅者用 NewEnvelope(Type, payload)
// 重新包一层再交给 Filter。
//
// Filter 接收包好的信封。返回 true 表示消息被消费（丢弃）；返回 false 表示
// 消息放行（Filter 自行决定是否广播 —— PlayerMoved 的 broadcastFilter 会
// 序列化信封并调 b.Broadcast）。nil Filter 等价于"始终放行但不做事"
// （仅用于纯日志/打点场景）。
type ChannelConfig struct {
	Channel string
	Type    string
	Filter  func(protocol.Envelope) bool
}

// RunMultiSubscriber 并发订阅所有 configs 中的频道。每个频道独立 goroutine、
// 独立重连循环、独立指数 backoff（1s → 5s 上限）。阻塞到 ctx 取消。
//
// Decode 错误（坏 JSON）只记日志并丢本条 —— 不关闭连接（一条坏数据不能毒化
// 整个频道）。
//
// ⚠️ ctx 必须是长寿命 appCtx，不能复用启动期的 10s 超时 ctx —— 否则 10s 后
// 所有订阅协程静默退出且不报任何错（api-gateway 踩过这个坑）。
func RunMultiSubscriber(ctx context.Context, rdb *goredis.Client, configs []ChannelConfig, logger *zap.Logger) error {
	if logger == nil {
		logger = zap.NewNop()
	}
	var wg sync.WaitGroup
	for _, cfg := range configs {
		wg.Add(1)
		go func(cfg ChannelConfig) {
			defer wg.Done()
			runChannelLoop(ctx, rdb, cfg, logger)
		}(cfg)
	}
	wg.Wait()
	return nil
}

// runChannelLoop 是单频道的"启动 → 跑 → 失败重连"主循环（沿用 Sprint 9
// 行为）。ctx 取消时干净退出。
func runChannelLoop(ctx context.Context, rdb *goredis.Client, cfg ChannelConfig, logger *zap.Logger) {
	logger.Info("redis subscriber starting",
		zap.String("channel", cfg.Channel),
		zap.String("type", cfg.Type))
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		err := runChannelOnce(ctx, rdb, cfg, logger)
		if err == nil || ctx.Err() != nil {
			return // ctx 取消引起的正常退出
		}
		logger.Warn("subscriber loop exited, will reconnect",
			zap.String("channel", cfg.Channel),
			zap.Error(err),
			zap.Duration("backoff", backoff))
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 5*time.Second {
			backoff *= 2
		}
	}
}

// runChannelOnce 一次 connect/subscribe 周期。ctx 取消或 pubsub 关闭时返 nil。
func runChannelOnce(ctx context.Context, rdb *goredis.Client, cfg ChannelConfig, logger *zap.Logger) error {
	pubsub := rdb.Subscribe(ctx, cfg.Channel)
	defer pubsub.Close()

	// 阻塞直到确认订阅成功（避免错过首批消息）
	if _, err := pubsub.Receive(ctx); err != nil {
		return err
	}
	logger.Info("redis subscriber ready",
		zap.String("channel", cfg.Channel),
		zap.String("type", cfg.Type))

	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-ch:
			if !ok {
				return nil
			}
			handleChannelMessage(cfg, msg.Payload, logger)
		}
	}
}

// handleChannelMessage 包信封 → 打 typed 日志 → 调 Filter。任何一步失败只
// 告警并丢这一条，不关闭连接。
func handleChannelMessage(cfg ChannelConfig, payload string, logger *zap.Logger) {
	env, err := protocol.NewEnvelope(cfg.Type, []byte(payload))
	if err != nil {
		// NewEnvelope 会因 raw 不是合法 JSON 而报错 —— 宁可丢一条，也别把坏
		// JSON 推给浏览器（web 端 JSON.parse 会整条丢弃且看不出哪层坏了）。
		logger.Warn("build envelope failed",
			zap.String("channel", cfg.Channel),
			zap.String("type", cfg.Type),
			zap.Error(err))
		return
	}

	logTypedReceive(cfg, env, logger)

	if cfg.Filter != nil {
		cfg.Filter(*env)
	}
}

// logTypedReceive 把 envelope.Payload 再 decode 进 typed struct，只为打
// Debug 日志；失败也只 warn（不影响转发）。
//
// 信封 payload 仍透传原始字节，避免 float32 精度在 round-trip 中漂移
// （见 protocol.NewEnvelope）。
func logTypedReceive(cfg ChannelConfig, env *protocol.Envelope, logger *zap.Logger) {
	switch cfg.Type {
	case protocol.TypePlayerMoved:
		var p protocol.PlayerMoved
		if err := json.Unmarshal(env.Payload, &p); err != nil {
			logger.Warn("decode player_moved inner failed",
				zap.String("channel", cfg.Channel), zap.Error(err))
			return
		}
		logger.Debug("player_moved received",
			zap.String("player_id", p.PlayerID),
			zap.String("tile_id", p.TileID))
	case protocol.TypeNpcDialogue:
		var n protocol.NpcDialogue
		if err := json.Unmarshal(env.Payload, &n); err != nil {
			logger.Warn("decode npc_dialogue inner failed",
				zap.String("channel", cfg.Channel), zap.Error(err))
			return
		}
		logger.Debug("npc_dialogue received",
			zap.String("npc_id", n.NpcID),
			zap.String("say", n.Say))
	default:
		logger.Debug("envelope received",
			zap.String("type", cfg.Type),
			zap.String("channel", cfg.Channel))
	}
}

// broadcastFilter 返回一个 Filter：marshal 信封 → 调 b.Broadcast → 返 false。
// marshal 失败时返 true（丢弃）并告警。
//
// PlayerMoved 内部用的 Filter；导出供单测复用。
func broadcastFilter(b Broadcaster, logger *zap.Logger) func(protocol.Envelope) bool {
	return func(env protocol.Envelope) bool {
		out, err := json.Marshal(env)
		if err != nil {
			logger.Warn("marshal envelope failed", zap.Error(err))
			return true
		}
		b.Broadcast(out)
		return false
	}
}

// PlayerMoved 保留 Sprint 9 API：异步启动一个 player_moved 频道订阅协程。
//
// ⚠️ ctx 必须是长寿命 appCtx，不能复用启动期的 10s 超时 ctx —— 否则 10s 后
// 订阅者静默退出且不报任何错（api-gateway 踩过这个坑）。
//
// 内部委托给 RunMultiSubscriber（单频道配置）。
func PlayerMoved(ctx context.Context, rdb *goredis.Client, channel string, b Broadcaster, logger *zap.Logger) {
	if logger == nil {
		logger = zap.NewNop()
	}
	go func() {
		_ = RunMultiSubscriber(ctx, rdb, []ChannelConfig{{
			Channel: channel,
			Type:    protocol.TypePlayerMoved,
			Filter:  broadcastFilter(b, logger),
		}}, logger)
	}()
}