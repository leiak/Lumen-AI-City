// Package redis 订阅 world-engine 的事件频道，包信封后交给 hub 扇出。
//
// 与 api-gateway/internal/subscriber/player_moved.go 是**两个独立订阅者**，
// 消费同一频道：api-gateway 写 PG player_position，这里推 WS。
// Redis pub/sub 天然支持多订阅者，两边互不影响。
package redis

import (
	"context"
	"encoding/json"
	"time"

	"github.com/aicity/ws-gateway/internal/protocol"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Broadcaster 是 hub.Hub 的最小接口（便于单测注入假实现）。
type Broadcaster interface {
	Broadcast(msg []byte)
}

// PlayerMoved 启动订阅协程；ctx 取消时退出。
//
// ⚠️ ctx 必须是长寿命的 appCtx，不能复用启动期的 10s 超时 ctx ——
// 否则 10s 后订阅者静默退出，且不报任何错（api-gateway 踩过这个坑）。
func PlayerMoved(ctx context.Context, rdb *goredis.Client, channel string, b Broadcaster, logger *zap.Logger) {
	logger.Info("redis subscriber starting", zap.String("channel", channel))

	go func() {
		backoff := time.Second
		for {
			if ctx.Err() != nil {
				return
			}
			err := runOnce(ctx, rdb, channel, b, logger)
			if err == nil || ctx.Err() != nil {
				return // ctx 取消引起的正常退出
			}
			logger.Warn("subscriber loop exited, will reconnect",
				zap.Error(err), zap.Duration("backoff", backoff))
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < 5*time.Second {
				backoff *= 2
			}
		}
	}()
}

func runOnce(ctx context.Context, rdb *goredis.Client, channel string, b Broadcaster, logger *zap.Logger) error {
	pubsub := rdb.Subscribe(ctx, channel)
	defer pubsub.Close()

	// 阻塞直到确认订阅成功（避免错过首批消息）
	if _, err := pubsub.Receive(ctx); err != nil {
		return err
	}
	logger.Info("redis subscriber ready", zap.String("channel", channel))

	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-ch:
			if !ok {
				return nil
			}
			handle(msg.Payload, b, logger)
		}
	}
}

// handle 解析 → 包信封 → 广播。任何一步失败只告警并丢这一条。
func handle(payload string, b Broadcaster, logger *zap.Logger) {
	// 解析只为拿到 player_id 打日志 + 挡掉坏 JSON；
	// 信封 payload 仍透传原始字节，避免 float32 精度在 round-trip 中漂移。
	var p protocol.PlayerMoved
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		logger.Warn("decode player_moved payload failed",
			zap.Error(err), zap.String("payload", payload))
		return
	}

	env, err := protocol.NewEnvelope(protocol.TypePlayerMoved, []byte(payload))
	if err != nil {
		logger.Warn("build envelope failed", zap.Error(err))
		return
	}
	out, err := json.Marshal(env)
	if err != nil {
		logger.Warn("marshal envelope failed", zap.Error(err))
		return
	}

	b.Broadcast(out)
	logger.Debug("player_moved broadcast",
		zap.String("player_id", p.PlayerID),
		zap.String("tile_id", p.TileID))
}
