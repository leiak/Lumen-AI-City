// Package main WebSocket Gateway（Sprint 9 + Sprint 11 多频道）
//
// 职责：Redis 多频道订阅（player_moved + npc_dialogue）→ 扇出到所有 WS 连接。
//
//	world-engine ─pub─> aicity:player:moved   ─sub─> api-gateway  (写 PG player_position)
//	                                        └sub─> ws-gateway
//	agent-os    ─pub─> aicity:npc_dialogue   ─sub─> ws-gateway
//
// 端点：
//   - GET /ws?token=<jwt>  WebSocket 升级（HS256，密钥与 api-gateway 共享）
//   - GET /healthz         liveness（含 hub stats）
//   - GET /readyz          readiness（Redis PING）
//
// 详细设计见 docs/04-API设计.md §18.3 + docs/11-技术细节与玩法模式.md §E.6
package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aicity/ws-gateway/internal/auth"
	"github.com/aicity/ws-gateway/internal/config"
	"github.com/aicity/ws-gateway/internal/cors"
	"github.com/aicity/ws-gateway/internal/hub"
	"github.com/aicity/ws-gateway/internal/protocol"
	wsredis "github.com/aicity/ws-gateway/internal/redis"
	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"nhooyr.io/websocket"
)

func main() {
	cfg := config.Load()
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	// 启动期独立短超时 ctx（不能用于长寿命订阅者）
	bootCtx, bootCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer bootCancel()

	redisOpt, err := goredis.ParseURL(cfg.RedisURL)
	if err != nil {
		logger.Fatal("redis url parse failed", zap.Error(err))
	}
	rdb := goredis.NewClient(redisOpt)
	defer rdb.Close()
	if err := rdb.Ping(bootCtx).Err(); err != nil {
		logger.Fatal("redis ping failed", zap.Error(err))
	}
	logger.Info("redis connected", zap.String("url", cfg.RedisURL))

	// 服务期 ctx：SIGINT/SIGTERM 时取消，hub 与订阅者才会干净退出。
	// ⚠️ 绝不能复用上面的 bootCtx（10s 后会自动取消，订阅者静默退出且不报错）。
	appCtx, appCancel := context.WithCancel(context.Background())
	defer appCancel()

	h := hub.New(logger)
	go h.Run(appCtx)

	// 多频道订阅（Sprint 11+ T02c）：player_moved + npc_dialogue 共用同一
	// hub.Broadcast 出口。每条消息到 hub 后再扇出到所有 WS 客户端。
	// broadcastFilter 闭包等价于 redis.broadcastFilter（后者 unexported）——
	// marshal 信封失败丢弃本条，否则把字节交给 hub 扇出。
	broadcastFilter := func(env protocol.Envelope) bool {
		publishEnvelope(h, logger, env)
		return false
	}
	go func() {
		// RunMultiSubscriber 是阻塞的（wg.Wait），ctx 取消时返回。包在 goroutine
		// 里让它和 PlayerMoved 旧版行为一致（fire-and-forget，靠 appCancel 关停）。
		if err := wsredis.RunMultiSubscriber(appCtx, rdb, []wsredis.ChannelConfig{
			{Channel: cfg.ChannelMoved, Type: protocol.TypePlayerMoved, Filter: broadcastFilter},
			{Channel: cfg.ChannelNpcDialogue, Type: protocol.TypeNpcDialogue, Filter: broadcastFilter},
			{Channel: cfg.ChannelNpcMoved, Type: protocol.TypeNpcMoved, Filter: broadcastFilter},
		}, logger); err != nil {
			logger.Error("multi-subscriber failed", zap.Error(err))
		}
	}()

	verifier := auth.NewVerifier(cfg.JWTSecret)

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", wsHandler(appCtx, cfg, h, verifier, logger))
	mux.HandleFunc("/healthz", healthzHandler(h))
	mux.HandleFunc("/readyz", readyzHandler(rdb))

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: cors.Middleware(cfg.CORSAllowedOrigins, mux),
		// WS 是长连接，不能设 Write/ReadTimeout（会掐断），只限制 header 读取阶段。
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("ws-gateway starting",
			zap.String("port", cfg.Port),
			zap.String("channel_moved", cfg.ChannelMoved),
			zap.String("channel_npc_dialogue", cfg.ChannelNpcDialogue),
			zap.String("channel_npc_moved", cfg.ChannelNpcMoved),
			zap.Bool("allow_anon", cfg.AllowAnon),
			zap.Bool("verify_origin", cfg.VerifyOrigin),
			zap.Int("send_buffer", cfg.SendBuffer),
			zap.Duration("ping_interval", cfg.PingInterval))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("listen failed", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info("shutting down...")

	// 先取消 appCtx：hub 断开所有连接、订阅者退出；再 graceful shutdown HTTP。
	// 顺序反了 srv.Shutdown 会一直等活着的 WS 连接直到超时。
	appCancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Warn("shutdown failed", zap.Error(err))
	}
	logger.Info("bye")
}

// publishEnvelope 把信封按类型分发：带非空 player_id 的 npc_dialogue 走
// SendToPlayer（只投目标玩家连接），其余（player_moved、主动广播）Broadcast 到所有。
// marshal 失败返回（丢弃本条）。抽成可测函数，broadcastFilter 与契约测试共用。
func publishEnvelope(b interface {
	Broadcast(msg []byte)
	SendToPlayer(playerID string, msg []byte)
}, logger *zap.Logger, env protocol.Envelope) {
	out, err := json.Marshal(env)
	if err != nil {
		logger.Warn("marshal envelope failed",
			zap.String("type", env.Type), zap.Error(err))
		return
	}
	if env.Type == protocol.TypeNpcDialogue {
		if playerID := npcDialogueTarget(env.Payload); playerID != "" {
			b.SendToPlayer(playerID, out)
			return
		}
	}
	b.Broadcast(out)
}

// npcDialogueTarget 从 npc_dialogue 负荷里取 player_id（"" = 主动广播，投递所有）。
// 解析失败按广播处理返回 ""：宁可多投，也不丢主动招呼。
func npcDialogueTarget(payload json.RawMessage) string {
	var d struct {
		PlayerID string `json:"player_id"`
	}
	if err := json.Unmarshal(payload, &d); err != nil {
		return ""
	}
	return d.PlayerID
}

// wsHandler 处理 /ws 升级：鉴权 → Accept → 注册 → 双 pump。
func wsHandler(
	appCtx context.Context,
	cfg *config.Config,
	h *hub.Hub,
	verifier *auth.Verifier,
	logger *zap.Logger,
) http.HandlerFunc {
	// Origin 校验开关。dev 默认关（WS_VERIFY_ORIGIN=false）；
	// 生产置 true 后按 CORS_ALLOWED_ORIGINS 匹配，与普通 HTTP 用同一份 allowlist。
	acceptOpts := &websocket.AcceptOptions{InsecureSkipVerify: true}
	if cfg.VerifyOrigin {
		acceptOpts = &websocket.AcceptOptions{
			OriginPatterns: cors.HostPatterns(cfg.CORSAllowedOrigins),
		}
	}

	return func(w http.ResponseWriter, r *http.Request) {
		// 鉴权在 Accept 之前：拒绝时回 HTTP 401 而不是先升级再关流，
		// 浏览器侧能直接看到状态码。
		playerID := ""
		claims, err := verifier.Verify(r.URL.Query().Get("token"))
		switch {
		case err == nil:
			playerID = claims.PlayerID
		case cfg.AllowAnon:
			logger.Warn("anonymous ws connection accepted (WS_ALLOW_ANON=true)",
				zap.Error(err))
		default:
			logger.Warn("ws auth rejected", zap.Error(err))
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		conn, err := websocket.Accept(w, r, acceptOpts)
		if err != nil {
			logger.Warn("ws accept failed", zap.Error(err))
			return
		}

		c := hub.NewClient(uuid.NewString(), playerID, conn, cfg.SendBuffer, logger)

		if err := h.Register(c); err != nil {
			c.Close(hub.StatusGoingAway, "server shutting down")
			return
		}
		defer func() {
			h.Unregister(c)
			c.Close(hub.StatusNormalClosure, "bye")
		}()

		// 连接生命周期挂在 appCtx 下：关停时两个 pump 一起退出。
		connCtx, connCancel := context.WithCancel(appCtx)
		defer connCancel()

		go c.WritePump(connCtx, cfg.PingInterval)
		// ReadPump 阻塞到对端断开（Sprint 9 无上行协议，但必须读才能感知 close 帧）
		c.ReadPump(connCtx)
	}
}

func healthzHandler(h *hub.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  "ok",
			"service": "ws-gateway",
			"hub":     h.Stats(),
		})
	}
}

func readyzHandler(rdb *goredis.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		redisOK := rdb.Ping(ctx).Err() == nil
		w.Header().Set("Content-Type", "application/json")
		if !redisOK {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ready": redisOK,
			"redis": redisOK,
		})
	}
}
