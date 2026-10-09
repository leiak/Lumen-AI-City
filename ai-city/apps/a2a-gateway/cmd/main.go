// Package main A2A Gateway 双协议入口（Sprint 5 + 5.5 + 6 + 7 + 7+ + 8）。
//
// 启动：监听 gRPC (A2A_GRPC_ADDR) + HTTP (A2A_HTTP_ADDR) 两个端口，
// 共用 *a2asrv.Service 单例；后台启动 a2a_inbox TTL cleanup cron。
//
// Sprint 7：注入 PG-backed CardStore + InboxStore；EchoAdapter 替换为 InboxAdapter。
// Sprint 7+：InboxStore 加 defaultTTL；启动 cron 周期 DELETE expires_at < NOW()。
// Sprint 8：注入 ACLStore → ACL 投递门（默认 allow；a2a_acl_policy 空表 = 全通）。
//
// 设计：docs/06-A2A协议.md §20；06-A2A-canonical.md（签名规范）。
//
// 环境变量：
//
//	A2A_GRPC_ADDR                    默认 127.0.0.1:50061
//	A2A_HTTP_ADDR                    默认 127.0.0.1:8083（HTTP gateway）
//	A2A_HTTP_API_KEY                 非空 = 启用 Bearer 鉴权（dev 留空）
//	A2A_REPLAY_WINDOW_SEC            ed25519 重放窗口秒数，默认 300
//	A2A_INBOX_TTL_HOURS              inbox 行 TTL 小时数，默认 168（7 天）；0 = 用 PG DEFAULT 兜底
//	A2A_INBOX_CLEANUP_INTERVAL_SEC   cleanup cron 间隔秒数，默认 300；0 = 禁用
//	A2A_ROUTING_TABLE                跨城 NPC 路由表 yaml 路径，默认 data/a2a-routing-table.yaml
//	CITY_ID                          本地城市 ID，默认 city_a
//	ECONOMY_SERVICE_URL              本地 economy-service 地址，默认 http://economy-service:8005
//	CROSS_CITY_INTERNAL_TOKEN        economy 内部跨城服务令牌
//	CROSS_CITY_RESERVATION_TTL       源城资金预留 TTL，默认 10m
//	BCITY_AGENT_OS_URL               B 城 agent-os 触发地址，默认 http://b-city:8081
//	REDIS_URL                        Redis 连接串（跨城 SayStream 订阅频道），默认 redis://redis:6379/0
//	DATABASE_URL                     PG 连接串（默认 postgresql://aicity:aicity_dev@localhost:5432/aicity）
package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/aicity/a2a-gateway/internal/a2asrv"
	"github.com/aicity/a2a-gateway/internal/crosscity"
	"github.com/aicity/a2a-gateway/internal/handlers"
	"github.com/aicity/a2a-gateway/internal/httpgw"
	"github.com/aicity/a2a-gateway/internal/router"
	a2av1 "github.com/aicity/proto/gen/go/a2a/v1"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
)

func main() {
	grpcAddr := getEnv("A2A_GRPC_ADDR", "127.0.0.1:50061")
	httpAddr := getEnv("A2A_HTTP_ADDR", "127.0.0.1:8083")
	apiKey := os.Getenv("A2A_HTTP_API_KEY")
	replaySec := parseReplayWindow()
	inboxTTL := parseInboxTTL()
	cleanupInterval := parseCleanupInterval()
	dbURL := getEnv("DATABASE_URL", "postgresql://aicity:aicity_dev@localhost:5432/aicity")
	routingTablePath := getEnv("A2A_ROUTING_TABLE", "data/a2a-routing-table.yaml")
	cityID := getEnv("CITY_ID", "city_a")
	economyURL := getEnv("ECONOMY_SERVICE_URL", "http://economy-service:8005")
	crossCityToken := getEnv("CROSS_CITY_INTERNAL_TOKEN", "dev-cross-city-service-token")
	reservationTTL := parseCrossCityReservationTTL()
	peerCityID := os.Getenv("PEER_CITY_ID")
	peerA2AEndpoint := os.Getenv("PEER_A2A_ENDPOINT")

	log.Printf("a2a-gateway starting: grpc=%s http=%s (replay_window=%ds api_key=%s db=%s inbox_ttl=%s cleanup_interval=%s routing_table=%s)",
		grpcAddr, httpAddr, int(replaySec.Seconds()), redactKey(apiKey), redactDSN(dbURL), inboxTTL, cleanupInterval, routingTablePath)

	// 跨城 NPC 路由表：启动期 fail-fast 加载；空表 = 联邦投递瘫痪。
	if err := router.LoadRoutes(routingTablePath); err != nil {
		log.Fatalf("load routing table: %v", err)
	}
	log.Printf("a2a-gateway: routing table loaded (size=%d)", len(router.GlobalTable.All()))

	// PG 连接（启动期 10s ctx；仿 api-gateway cmd/main.go:34）
	bootCtx, bootCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer bootCancel()
	pool, err := pgxpool.New(bootCtx, dbURL)
	if err != nil {
		log.Fatalf("pg connect: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(bootCtx); err != nil {
		log.Fatalf("pg ping: %v", err)
	}
	cardStore := a2asrv.NewCardStore(pool)
	inboxStore := a2asrv.NewInboxStoreWithTTL(pool, inboxTTL)
	// Sprint 8：ACL deny list（默认 allow —— a2a_acl_policy 空表 = 全通）
	aclStore := a2asrv.NewACLStore(pool)
	acl := a2asrv.NewACL(aclStore)
	log.Printf("a2a-gateway: PG ready (card_store + inbox_store + acl_store wired)")

	// 共享 Service
	reg := a2asrv.NewRegistryFromCardStore(cardStore)
	verifier := a2asrv.NewVerifier(replaySec)
	dispatcher := a2asrv.NewDispatcher()

	// InboxAdapter：aicity provider 的兜底；也作为 Dispatcher fallback
	inboxAdapter := a2asrv.NewInboxAdapter(inboxStore)

	// Outbound HTTP adapter：openclaw / workbuddy（共享 5s timeout client + inbox fallback）
	httpClient := a2asrv.NewHTTPClient(5 * time.Second)
	dispatcher.Register(a2asrv.NewHTTPAdapter("openclaw", "openclaw", httpClient, inboxStore))
	dispatcher.Register(a2asrv.NewHTTPAdapter("workbuddy", "workbuddy", httpClient, inboxStore))
	dispatcher.Register(inboxAdapter)
	dispatcher.SetFallback(inboxAdapter)

	svc := a2asrv.NewService(reg, verifier, dispatcher, inboxStore, acl)

	// B1：跨城流式 MirrorStore（60min TTL —— 与 stage 2 SessionStore 一致）。
	// 注入到 Service 后，SayStreamForward init 帧会 Create session，正常结束
	// 路径 defer MarkDone(complete=true)；T06 失败路径传 false。
	mirrorStore := crosscity.NewMirrorStore(60 * time.Minute)
	svc.SetMirrorStore(mirrorStore)
	moneyClient := crosscity.NewHTTPMoneyClient(
		economyURL,
		crossCityToken,
		&http.Client{Timeout: 5 * time.Second},
	)
	svc.SetMoneyClient(moneyClient, cityID)
	var stopReconciler context.CancelFunc = func() {}
	if peerCityID != "" && peerA2AEndpoint != "" {
		peerConn, peerErr := crosscity.Dial(peerA2AEndpoint)
		if peerErr != nil {
			log.Fatalf("peer a2a dial: %v", peerErr)
		}
		defer peerConn.Close()
		reconcilerInterval := 30 * time.Second
		if value, err := time.ParseDuration(getEnv("CROSS_CITY_RECONCILE_INTERVAL", "30s")); err == nil && value > 0 {
			reconcilerInterval = value
		}
		stopReconciler = crosscity.StartCrossCityReconciler(context.Background(), &crosscity.Reconciler{
			Local:      moneyClient,
			Remote:     crosscity.NewGRPCMoneyGateway(peerConn),
			PeerCityID: peerCityID,
		}, reconcilerInterval)
		log.Printf("a2a-gateway: CrossCityReconciler started (peer=%s interval=%s)", peerCityID, reconcilerInterval)
	} else {
		log.Printf("a2a-gateway: CrossCityReconciler disabled (PEER_CITY_ID/PEER_A2A_ENDPOINT unset)")
	}
	defer stopReconciler()
	log.Printf("a2a-gateway: MirrorStore wired (ttl=60m)")
	log.Printf("a2a-gateway: CrossCityMoney wired (city=%s economy=%s ttl=%s)", cityID, economyURL, reservationTTL)

	// B1-T06：跨城 SayStreamForwarder（A 城 a2a-gateway → B 城 agent-os）。
	//
	// B1-T06 followup：注入 *crosscity.RealSubscriber（直包 *redis.Client）。
	// Redis URL 走 REDIS_URL env（与 api-gateway/ws-gateway 同步：默认 redis://redis:6379/0）。
	redisURL := getEnv("REDIS_URL", "redis://redis:6379/0")
	redisOpt, redisErr := redis.ParseURL(redisURL)
	if redisErr != nil {
		log.Fatalf("redis url parse %s: %v", redisURL, redisErr)
	}
	redisClient := redis.NewClient(redisOpt)
	defer redisClient.Close()
	if err := redisClient.Ping(bootCtx).Err(); err != nil {
		log.Fatalf("redis ping %s: %v", redisURL, err)
	}
	log.Printf("a2a-gateway: redis connected (url=%s)", redisURL)

	bCityURL := getEnv("BCITY_AGENT_OS_URL", "http://b-city:8081")
	bCityClient := crosscity.NewBCityClient(bCityURL)
	forwarder := crosscity.NewForwarder(bCityClient, crosscity.NewRealSubscriber(redisClient), mirrorStore)
	log.Printf("a2a-gateway: Forwarder wired (b_city=%s subscriber=real channel=%s)", bCityURL, forwarder.Channel)

	// gRPC server
	grpcLis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		log.Fatalf("grpc listen %s: %v", grpcAddr, err)
	}
	grpcSrv := grpc.NewServer()
	a2av1.RegisterA2AGatewayServer(grpcSrv, svc)

	// HTTP server
	httpHandler := httpgw.New(svc, apiKey)
	// Task 54：跨城 NPC 对话路由（POST /v1/cross_city/talk/:npc_id）
	// 走 GlobalTable + crosscity.Dial(mTLS) 转发到远端 world-engine。
	// 直接挂到 httpgw 暴露的 gin.Engine 上，与其它路由共用中间件链（trace_id /
	// recovery / logging / 可选 Bearer auth）。
	httpHandler.Engine().POST("/v1/cross_city/talk/:npc_id", handlers.CrossCityTalkHandler(router.GlobalTable))

	// B1-T06：挂载 POST /v1/federation/say_stream（B1 跨城流式入口）。
	// forwarder 当前接 RealSubscriber —— Redis 链路打通，SSE endpoint 真实可用。
	httpHandler.Engine().POST("/v1/federation/say_stream", gin.WrapH(&httpgw.SayStreamHandler{
		APIKey: apiKey,
		Client: forwarder,
	}))

	// B1-T08：挂载 GET /v1/federation/sessions/:sid/buffer。
	// 客户端断线 / 重连后可调用此 endpoint 从 MirrorStore 拉 from_idx 起的
	// beats 实现 replay。SessionReplayHandler 内置 Bearer 鉴权 + 手动 URL
	// 切片（fallback for gin.WrapH 不写 PathValue 的限制）。
	httpHandler.Engine().GET("/v1/federation/sessions/:sid/buffer", gin.WrapH(&httpgw.SessionReplayHandler{
		APIKey: apiKey,
		Mirror: mirrorStore,
	}))
	httpSrv := &http.Server{
		Addr:              httpAddr,
		Handler:           httpHandler.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Sprint 7+：启动 inbox TTL cleanup 后台 cron（独立 long-lived ctx 模式；
	// 与 api-gateway subscriber 同坑 —— 不能复用启动期 bootCtx 否则 10s 后 ctx 自动取消）
	a2asrv.StartInboxCleanup(ctx, inboxStore, cleanupInterval)

	var wg sync.WaitGroup

	// gRPC serve goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Printf("a2a-gateway gRPC ready on %s", grpcAddr)
		if err := grpcSrv.Serve(grpcLis); err != nil {
			log.Printf("grpc.Serve: %v", err)
		}
	}()

	// HTTP serve goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Printf("a2a-gateway HTTP ready on %s (api_key=%s)", httpAddr, redactKey(apiKey))
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("http.Serve: %v", err)
		}
	}()

	log.Printf("a2a-gateway ready (registry size=%d)", reg.Size())

	<-ctx.Done()
	log.Printf("a2a-gateway shutting down ...")
	stopReconciler()

	// 双协议并行 graceful shutdown（HTTP 5s 限时）
	httpShutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	httpStopped := make(chan struct{})
	go func() {
		if err := httpSrv.Shutdown(httpShutdownCtx); err != nil {
			log.Printf("http.Shutdown: %v", err)
		}
		close(httpStopped)
	}()

	grpcStopped := make(chan struct{})
	go func() {
		grpcSrv.GracefulStop()
		close(grpcStopped)
	}()

	// 等两个 server 都停（或超时）
	select {
	case <-grpcStopped:
		log.Printf("gRPC stopped")
	case <-httpShutdownCtx.Done():
		log.Printf("HTTP shutdown timeout, force stop")
		grpcSrv.Stop()
	}
	<-httpStopped
	log.Printf("HTTP stopped")

	// 等 serve goroutine 退出（理论上 Serve 已返）
	wg.Wait()
	log.Printf("a2a-gateway stopped cleanly")
}

// getEnv 读 env，空返 fallback。
func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// parseReplayWindow 读 A2A_REPLAY_WINDOW_SEC；非法/缺失 → 5min。
func parseReplayWindow() time.Duration {
	s := os.Getenv("A2A_REPLAY_WINDOW_SEC")
	if s == "" {
		return 5 * time.Minute
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		log.Printf("a2a-gateway: invalid A2A_REPLAY_WINDOW_SEC=%q, fallback to 300s", s)
		return 5 * time.Minute
	}
	return time.Duration(n) * time.Second
}

// parseInboxTTL 读 A2A_INBOX_TTL_HOURS；非法/缺失 → 168h（7 天）。
//
//   - n == 0 → 返 0 表示"PG DEFAULT 兜底"（不显式写 expires_at）
//   - n  < 0 → log + fallback
//   - 非法字符串 → log + fallback
func parseInboxTTL() time.Duration {
	s := os.Getenv("A2A_INBOX_TTL_HOURS")
	if s == "" {
		return 168 * time.Hour
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		log.Printf("a2a-gateway: invalid A2A_INBOX_TTL_HOURS=%q, fallback to 168h", s)
		return 168 * time.Hour
	}
	return time.Duration(n) * time.Hour
}

func parseCrossCityReservationTTL() time.Duration {
	value, err := time.ParseDuration(getEnv("CROSS_CITY_RESERVATION_TTL", "10m"))
	if err != nil || value <= 0 {
		log.Printf("a2a-gateway: invalid CROSS_CITY_RESERVATION_TTL, fallback to 10m")
		return 10 * time.Minute
	}
	return value
}

// parseCleanupInterval 读 A2A_INBOX_CLEANUP_INTERVAL_SEC；非法/缺失 → 300s。
//
//   - n == 0 → 返 0 表示"禁用 cron"（StartInboxCleanup 会 log）
//   - n  < 0 → log + fallback
func parseCleanupInterval() time.Duration {
	s := os.Getenv("A2A_INBOX_CLEANUP_INTERVAL_SEC")
	if s == "" {
		return 300 * time.Second
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		log.Printf("a2a-gateway: invalid A2A_INBOX_CLEANUP_INTERVAL_SEC=%q, fallback to 300s", s)
		return 300 * time.Second
	}
	return time.Duration(n) * time.Second
}

// redactKey 把 api key 缩成 "set" / "unset" 形式打印（避免日志泄露密钥）。
func redactKey(k string) string {
	if k == "" {
		return "unset"
	}
	return "set"
}

// redactDSN 把 PG 连接串的密码部分打码（保留 user@host:port/db）。
func redactDSN(dsn string) string {
	// 简化：仅保留 scheme://user:***@host:port/db 形式
	// 完整 DSN 解析交给 pgx；这里只脱敏
	if i := indexOf(dsn, "://"); i >= 0 {
		rest := dsn[i+3:]
		if at := indexOf(rest, "@"); at >= 0 {
			userHost := rest[:at]
			hostPart := rest[at+1:]
			// user:pass → user:***
			if colon := indexOf(userHost, ":"); colon >= 0 {
				return dsn[:i+3] + userHost[:colon+1] + "***@" + hostPart
			}
		}
	}
	return dsn
}

// indexOf 简化版 strings.Index（避免引入 strings import 噪声）。
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
