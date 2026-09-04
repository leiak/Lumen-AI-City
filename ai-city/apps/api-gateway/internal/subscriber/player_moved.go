// Package subscriber 订阅 Redis 频道，把 world-engine 事件落库。
//
// 订阅 `aicity:player:moved` → 解析 PlayerPosition → 调用 PlayerStore.UpsertPosition。
// 失败仅日志告警，不退出循环（消息自然丢失不影响后续事件）。
package subscriber

import (
	"context"
	"encoding/json"
	"time"

	"github.com/aicity/api-gateway/internal/store"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// PlayerMovedPayload 是 world-engine 序列化进 Redis 的消息体。
// 字段名需与 `world-engine/src/rest.rs` 里 PlayerPosition 的 serde 输出一致。
type PlayerMovedPayload struct {
	PlayerID string  `json:"player_id"`
	TileID   string  `json:"tile_id"`
	X        float32 `json:"x"`
	Y        float32 `json:"y"`
	TsMs     int64   `json:"ts_ms"`
}

// PlayerMoved 启动订阅协程；ctx 取消时退出。
// rdb 已建立的连接复用即可（也支持新连接）。
func PlayerMoved(ctx context.Context, rdb *redis.Client, channel string, players *store.PlayerStore, logger *zap.Logger) {
	logger.Info("redis subscriber starting", zap.String("channel", channel))

	go func() {
		// 失败自动重连：退避 1s → 5s
		backoff := time.Second
		for {
			if ctx.Err() != nil {
				return
			}
			if err := runOnce(ctx, rdb, channel, players, logger); err != nil && ctx.Err() == nil {
				logger.Warn("subscriber loop exited, will reconnect",
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
				continue
			}
			// ctx 取消引起的退出
			return
		}
	}()
}

func runOnce(ctx context.Context, rdb *redis.Client, channel string, players *store.PlayerStore, logger *zap.Logger) error {
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
			var p PlayerMovedPayload
			if err := json.Unmarshal([]byte(msg.Payload), &p); err != nil {
				logger.Warn("decode player_moved payload failed",
					zap.Error(err),
					zap.String("payload", msg.Payload))
				continue
			}
			if err := players.UpsertPosition(ctx, p.PlayerID, p.TileID, p.X, p.Y); err != nil {
				logger.Warn("upsert player_position failed",
					zap.Error(err),
					zap.String("player_id", p.PlayerID),
					zap.String("tile_id", p.TileID))
				continue
			}
			logger.Debug("player_position upserted",
				zap.String("player_id", p.PlayerID),
				zap.String("tile_id", p.TileID),
				zap.Float32("x", p.X),
				zap.Float32("y", p.Y))
		}
	}
}
