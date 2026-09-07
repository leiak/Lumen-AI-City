# Sprint 9 复盘

> 范围：**ws-gateway 实装 + web 切轮询 —— 砍掉 WorldMap 3s 滞后**
>
> 完成时间：2026-09-07
>
> 提交：
> - `65ceec7` feat(sprint9): ws-gateway 实装 + web 切轮询

## 一、本次交付

Sprint 8 把 web 真客户端链路打通，但 WorldMap 仍是 `setInterval(fetchTiles, 3000)`：
自己移动要等下一次 tick 才看得到，其它玩家的移动也只能 3s 一帧。
Sprint 9 把这个 3s 滞后砍掉 —— 新增第 7 个容器 `ws-gateway`，订阅 Redis 频道 `aicity:player:moved`，
实时 fanout 到所有 WS 连接；web 端把轮询换成 WS 事件 + 200ms debounce refetch。

```
world-engine ─pub─> Redis ─sub─> api-gateway    (写 PG player_position，保留不动)
                  └─sub─> ws-gateway ─ws─> web   (新增)
```

### A. ws-gateway 服务（Go 1.23）

| 项 | 说明 |
|---|---|
| 镜像 | `aitown-ws-gateway`，多阶段 build（builder golang:1.23-alpine + final alpine + ca-certificates + tzdata），同时编出 `ws-gateway` 主进程和 `ws_smoke` 端到端冒烟两个二进制 |
| 端口 | `0.0.0.0:8082`，端点 `/ws?token=<jwt>` / `/healthz` / `/readyz`（Redis PING） |
| 鉴权 | `?token=` 走 HS256 验签（与 api-gateway 共享 `JWT_SECRET`），`jwt.WithValidMethods(["HS256"])` 显式拒 `alg=none`；`WS_ALLOW_ANON=true` 才放空 token（仅 dev）|
| CORS | 镜像 `apps/api-gateway/internal/middleware/cors.go`：allowlist 回显 Origin（不用 `*`）+ Vary:Origin 恒发 + 预检 204/403 + Allow-Headers 含 Authorization |
| 协议 | Client→Server 无上行（仅连接即订阅 + `?token=` 鉴权）；Server→Client 唯一 envelope `{type, trace_id, ts_ms, payload}` |
| hub 模式 | 单 goroutine 拥有 `clients map[*Client]struct{}`，三 chan `register/unregister/broadcast`（与 nhooyr 示例一致；写无锁）|
| 背压 | send chan 缓冲 16（`WS_SEND_BUFFER`）；非阻塞 trySend 失败 → `Close(StatusPolicyViolation, "slow consumer")` 驱逐；`Close` 用 `sync.Once` 幂等 |
| shutdown | SIGINT → `appCancel()` 先（让 hub drain 所有 client）→ `srv.Shutdown(30s)`；反序会卡满 30s 等活连接 |
| 环境变量 | `WS_GATEWAY_PORT`(8082) / `REDIS_URL` / `REDIS_CHANNEL_MOVED`(aicity:player:moved) / `JWT_SECRET` / `WS_ALLOW_ANON` / `WS_VERIFY_ORIGIN`(prod true) / `WS_SEND_BUFFER`(16) / `WS_PING_INTERVAL_SEC`(30) / `CORS_ALLOWED_ORIGINS` |

### B. 包结构（与 api-gateway 对齐）

```
apps/ws-gateway/
  cmd/
    main.go         ← 重写（替换 62 行 stub）
    ws_smoke/main.go  ← 端到端冒烟（6 步）
  internal/
    config/config.go          (env → struct)
    auth/jwt.go                (HS256 verify)
    cors/cors.go               (preflight + Vary:Origin)
    hub/{hub.go,client.go}     (经典 hub pattern)
    redis/subscriber.go        (port api-gateway/subscriber/player_moved.go)
    protocol/message.go        (envelope + PlayerMoved)
```

**关键决策：两订阅者独立**。api-gateway 的 `subscriber.PlayerMoved` 继续写 PG，ws-gateway 用
同样的模式 fanout 到 WS。两个独立 Redis conn，互不影响（Redis pub/sub 多订阅者天然支持，
fd 翻倍可接受）。Sprint 9 MVP 不合并，避免对 api-gateway 写路径造成回归风险。

### C. web 切轮询

| 项 | 说明 |
|---|---|
| 新文件 | `web/src/lib/ws-events.ts`：`startWsBridge()` 把 WS 帧桥到 game store + `CustomEvent('aicity:player_moved')` |
| WorldMap | 删 `setInterval(fetchTiles, POLL_MS)`；改 `window.addEventListener('aicity:player_moved', debouncedRefetch, 200ms)`；保留 mount 时首次 `fetchTiles()` 兜底 |
| city/page | `useEffect(() => startWsBridge(), [])` —— 进 city 起桥 |
| `web/src/lib/ws.ts` | **不动**（auto-reconnect 1s→30s 退避 + OfflineQueue 已经够用） |
| PlayerHUD | 不动（store 已实时更新）|

**为什么用 CustomEvent 桥**：WS 帧落到 game store（自己位置校准）+ `CustomEvent` 通知
WorldMap 重新 fetchTiles（其它玩家 tile 切换需要新数据）。CustomEvent 是浏览器原生
跨组件通信；不引 RxJS / EventEmitter / 全局 store 副作用。

### D. ws_smoke E2E

`apps/ws-gateway/cmd/ws_smoke/main.go`（6 步，bake 进镜像）：
1. `POST /v1/auth/login`（demo/demo123）→ token + player_id
2. `websocket.Dial("ws://127.0.0.1:8082/ws?token="+token)`
3. 单独 dial 不带 token → 期望被拒（401）
4. `POST /v1/world/move` 触发 demo 移动到 tile_0_0 (50, 50)
5. 在 WS 上等 `player_moved` envelope（5s 超时）
6. 验 `payload.player_id === player_id` + `payload.x ≈ 50`

跑法：`docker compose exec -T ws-gateway /app/ws_smoke`，exit 0 即过。

### E. Playwright WS 推送测试

`web/e2e/ws-push.spec.ts`：用 `page.addInitScript` 包装 `window.WebSocket`，
挂 `addEventListener('message')` 解析 `e.data` 写 `window.__wsFrames`。
进 `/city` 后断言：
1. `window.__wsOpened > 0`（WS 真连上）
2. dispatch `tile_-1_0` 中心的 click 后，5s 内收到 `player_moved` envelope
3. envelope 字段齐全：`trace_id` 非空 + `ts_ms > 0` + `payload.tile_id === 'tile_-1_0'`

比断言 DOM 更直接 —— DOM 更新还要过 debounce + fetch；直接断言 WS 帧是 ground truth。

## 二、E2E 输出

```bash
$ docker compose ps
NAME                    IMAGE                 STATUS                    PORTS
aitown-a2a-gateway-1    aitown-a2a-gateway    Up 5 hours (healthy)      0.0.0.0:8083->8083, 0.0.0.0:50061->50061
aitown-api-gateway-1    aitown-api-gateway    Up 5 hours (healthy)      0.0.0.0:8080->8080
aitown-postgres-1       postgres:16-alpine    Up 5 hours (healthy)      127.0.0.1:5432->5432
aitown-redis-1          redis:7-alpine        Up 5 hours (healthy)      127.0.0.1:6379->6379
aitown-web-1            aitown-web            Up About an hour          0.0.0.0:3000->3000
aitown-world-engine-1   aitown-world-engine   Up 5 hours (healthy)      0.0.0.0:50051-50052
aitown-ws-gateway-1     aitown-ws-gateway     Up 28 minutes (healthy)   0.0.0.0:8082->8082

$ cd apps/ws-gateway && go test ./... -count=1
?   	github.com/aicity/ws-gateway/cmd	[no test files]
?   	github.com/aicity/ws-gateway/cmd/ws_smoke	[no test files]
ok  	github.com/aicity/ws-gateway/internal/auth	0.084s
?   	github.com/aicity/ws-gateway/internal/config	[no test files]
ok  	github.com/aicity/ws-gateway/internal/cors	2.282s
ok  	github.com/aicity/ws-gateway/internal/hub	0.209s
ok  	github.com/aicity/ws-gateway/internal/protocol	0.083s
ok  	github.com/aicity/ws-gateway/internal/redis	0.161s

$ docker compose exec -T ws-gateway /app/ws_smoke
[OK]   1/6 login user=demo player_id=1fce8ddd-2f3e-40ab-bd23-75ede603e98f
[OK]   2/6 ws connected ws://127.0.0.1:8082/ws
[OK]   3/6 ws without token rejected
[OK]   4/6 move accepted tile=tile_0_0 pos=(50.0,50.0)
[OK]   5/6 player_moved received trace_id=7ffca638-ee8b-4143-b202-e9bd89c625cc ts_ms=1788764052689
[OK]   6/6 payload player_id=1fce8ddd-2f3e-40ab-bd23-75ede603e98f tile_id=tile_0_0 pos=(50.0,50.0)
[OK] all 6 ws_smoke checks passed (api=http://api-gateway:8080 ws=ws://127.0.0.1:8082/ws)

$ cd web && pnpm exec playwright test
Running 3 tests using 1 worker
initial HUD: x=50, y=50
will click tile_-1_0 center (-50, 50)
  ✓  1 [chromium] › e2e\map-flow.spec.ts:11:5 › login → /city → 9 tiles visible → click move → position changed (4.4s)
  ✓  2 [chromium] › e2e\map-flow.spec.ts:140:5 › login failure shows error message (537ms)
  ✓  3 [chromium] › e2e\ws-push.spec.ts:42:5 › 进 /city 后 WS 连上，移动触发 player_moved 推送 (626ms)
3 passed (6.7s)

$ curl -s http://localhost:8082/healthz | jq .
{
  "status": "ok",
  "service": "ws-gateway",
  "hub": { "clients": 0, "register_total": 0, "unregister_total": 0, "broadcast_total": 0 }
}
```

## 三、关键决策

| # | 决策 | 理由 |
|---|---|---|
| 1 | 两独立 Redis 订阅者（api-gateway + ws-gateway）| Redis pub/sub 多订阅者天然支持；写路径不动降低回归风险；Sprint 10+ 再合并 |
| 2 | 单 goroutine hub pattern | 与 nhooyr 示例一致；写无锁；channel + select 是 idiomatic Go |
| 3 | Client send chan 缓冲 16 + 慢消费者驱逐 | 慢客户端不能拖死 hub；`StatusPolicyViolation` 是 RFC 6455 标准 close code |
| 4 | WS 鉴权走 `?token=<jwt>` query string | 与现有 `web/src/lib/ws.ts` 实现对齐；HS256 与 api-gateway 共用密钥 |
| 5 | CORS 单独包（`internal/cors`） | 镜像 api-gateway gin 版；7 case 测试守住（Allow-Headers 含 Authorization + Vary 恒发 + 预检 204/403）|
| 6 | `WS_VERIFY_ORIGIN=false` (dev) / `true` (prod) | dev 用 `InsecureSkipVerify: true` 接受任何 origin；prod 切 `OriginPatterns` allowlist。Sprint 9 保留 dev 现状 |
| 7 | nhooyr `OriginPatterns` 必须用 `cors.HostPatterns(allowed)` 剥 scheme | nhooyr `authenticateOrigin` 拿 `url.Parse(origin).Host` 匹配 pattern；带 scheme 的 pattern 永远匹配不上，**症状是 403 而不是配置报错** |
| 8 | boot ctx (10s) vs appCtx (long-lived) 严格分离 | 10s 后 ctx 自动取消，订阅者静默退出无报错 —— 已知 api-gateway 同款坑 |
| 9 | shutdown 顺序：`appCancel()` 先 → `srv.Shutdown(30s)` 后 | 让 hub 先 drain 所有 client；反序 Shutdown 卡满 30s 等活连接 |
| 10 | payload 字段名必须是 `player_id` | Rust serde 输出（`apps/world-engine/src/world_grid.rs:25`）；gRPC proto 里的 `entity_id` 只在 proto 内部使用 |
| 11 | JSON envelope 而非 protobuf over WS | WS 上行是文本协议，protobuf 没收益；envelope 字段稳定后可单独升 v2 |
| 12 | CustomEvent 而非 prop drilling / RxJS | 浏览器原生，零依赖；WorldMap 用 `window.addEventListener` 收 + debounce 200ms refetch |
| 13 | 200ms debounce refetch | 多玩家同时移动不会触发风暴；折中"近实时"和"不浪费 fetch" |
| 14 | ws_smoke bake 进镜像 | 与 a2a_smoke / http_smoke 同款；compose exec 即跑，CI 友好 |
| 15 | Playwright 嗅探用 `addInitScript` 包装 WebSocket | 比断言 DOM 更直接（DOM 还要过 debounce + fetch）；保留原生 WS 行为 |
| 16 | JWT secret 共享走 compose 同一 `JWT_SECRET` env 块 | 避免漂移；一个值维护 |
| 17 | MVP 全广播，不过滤 tile/region | 9 tile × 几玩家量级；过滤是 Sprint 10+ 优化 |
| 18 | 错误码不用 protocol buffer enum | HTTP 401 / 403 足以表达；envelope 里只走 happy path |

## 四、变更清单

### 新增（16）

ws-gateway：
- `apps/ws-gateway/cmd/ws_smoke/main.go` —— 6 步端到端冒烟
- `apps/ws-gateway/internal/config/config.go` —— env → struct
- `apps/ws-gateway/internal/auth/jwt.go` + `jwt_test.go` —— HS256 verify（4 case）
- `apps/ws-gateway/internal/cors/cors.go` + `cors_test.go` —— net/http 中间件 + HostPatterns（7+4 case）
- `apps/ws-gateway/internal/hub/hub.go` + `hub_test.go` —— 单 goroutine hub（4 case，含 race）
- `apps/ws-gateway/internal/hub/client.go` + `client_test.go` —— Client WritePump/ReadPump/Close（3 case）
- `apps/ws-gateway/internal/hub/fakeconn_test.go` —— 测试用 fake websocket.Conn
- `apps/ws-gateway/internal/redis/subscriber.go` + `subscriber_test.go` —— port api-gateway 订阅者
- `apps/ws-gateway/internal/protocol/message.go` + `message_test.go` —— Envelope 序列化 + RawMessage passthrough（2 case）
- `apps/ws-gateway/go.sum` —— 依赖 lock

web：
- `web/src/lib/ws-events.ts` —— `startWsBridge()` 桥 WS 帧 → game store + CustomEvent
- `web/e2e/ws-push.spec.ts` —— Playwright WS 推送测试（addInitScript 嗅探）

workspace：
- `apps/a2a-gateway/go.sum` —— workspace 必需（缺失 fail）
- `apps/observability-agent/go.sum` —— workspace 必需

### 修改（14）

- `apps/ws-gateway/Dockerfile` —— workspace-aware 多阶段 build（同时编 ws-gateway + ws_smoke）
- `apps/ws-gateway/cmd/main.go` —— 重写 62 行 stub 为完整服务（signal → appCancel → Shutdown）
- `apps/ws-gateway/go.mod` —— drop `segmentio/kafka-go`，add `golang-jwt/jwt/v5` + `zap` + `google/uuid`
- `apps/api-gateway/go.mod` / `go.sum` —— `go mod tidy` 副作用（grpc 提为 direct + gin 树补全）
- `apps/a2a-gateway/go.mod` —— 同上
- `apps/observability-agent/go.mod` —— 同上
- `packages/proto/go.mod` / `go.sum` —— `golang.org/x/{sys,text}` 版本 bump
- `docker-compose.yml` —— 加 `ws-gateway` 第 7 容器；`depends_on: redis(healthy) + api-gateway(started)`
- `go.work` / `go.work.sum` —— 重排 use 顺序
- `web/src/components/Map/WorldMap.tsx` —— 删 `setInterval(fetchTiles, 3000)` + 加 CustomEvent listener + debounce refetch
- `web/src/app/city/page.tsx` —— `useEffect(() => startWsBridge(), [])`

### 删除（0）

无 —— 完全是增量。Sprint 8 的 `setInterval` 也没真删（被替换成新逻辑）。

## 五、关键陷阱

| # | 陷阱 | 缓解 |
|---|---|---|
| 1 | **hub 关闭顺序：`close(h.done)` 必须在关闭 clients 之前**| 用 defer 会在 clients 关完之后才 close(done)，并发 `Register` 拿不到 done 信号。改：ctx.Done 分支第一行 `close(h.done)`；race detector 立即暴露 |
| 2 | Register/Unregister/Broadcast 必须**两步 select** | 第一步 `select { case <-h.done: err; default: }`；第二步 `select { case h.register <- c: ; case <-h.done: err }`。单步 select 拿不到 done 信号 |
| 3 | **nhooyr `OriginPatterns` 匹配 host 不是 origin** | 必须用 `cors.HostPatterns(allowed)` 剥 scheme —— `url.Parse("http://x:3000").Host == "x:3000"`。否则 prod 所有跨源 WS 403 且不报错。`TestHostPatterns` 守住 |
| 4 | **boot ctx vs appCtx 分离** | 复用 10s boot ctx 给订阅者，10s 后 ctx 自动取消，订阅者静默退出无报错。`hub.Run(ctx)` 必须用 long-lived appCtx |
| 5 | **shutdown 顺序反了 Shutdown 卡满 30s** | `appCancel()` 先（让 hub drain 完所有 client）→ `srv.Shutdown(30s)`；顺序反了会等所有活 WS 连接超时 |
| 6 | **payload 字段名：必须是 `player_id` 不是 `entity_id`** | Rust serde 输出是 `player_id`；proto 里的 `entity_id` 只在 proto 内部。改错后 ws_smoke 6/6 立刻 fail |
| 7 | **`alg=none` JWT 攻击** | `jwt.ParseWithClaims` 不显式 `WithValidMethods(["HS256"])` 会接受 unsigned token；`jwt_test.go` 4 case 守住（含错签名/过期/格式错）|
| 8 | CORS allowlist 空串/空白项 | `strings.TrimSpace` 过滤；否则 `CORS_ALLOWED_ORIGINS` 末尾多个逗号就会让空 Origin 分支放行。`TestCORS_BlankOriginsIgnored` 守住 |
| 9 | **`Access-Control-Allow-Headers` 必须含 Authorization** | 不含则鉴权路由预检失败；`TestCORS_PreflightHeaders` 守住 |
| 10 | **`Vary: Origin` 必须恒发** | 否则共享缓存把一个 origin 的响应喂给另一个 origin；`TestCORS_VaryOriginAlwaysSetWhenOriginPresent` 守住 |
| 11 | Client `Close` 必须 `sync.Once` 幂等 | 双 pump 关闭时都会调 Close；不用 sync.Once 会 panic "close of closed channel"。`client_test.go` 守住 |
| 12 | ws-gateway 容器必须等 Redis healthy | `depends_on: redis: condition: service_healthy`；不等 Redis 启动会 `logger.Fatal("redis ping failed")` 一闪而过 |
| 13 | api-gateway 不卡 healthy | `depends_on: api-gateway: condition: service_started`；不卡 healthy 防启动链过长（只为让 JWT_SECRET 生效顺序直观）|
| 14 | **`MSYS_NO_PATHCONV=1` docker compose exec** | 不设的话 `/app/ws_smoke` 被 MSYS 转成 `C:/Program Files/Git/app/ws_smoke`；"exec failed: no such file or directory" 莫名报错 |
| 15 | Playwright `getByLabel` 失败 | login 页 input 没绑 `htmlFor`；改 `page.locator('input[type="text"]').first().fill(...)`。与 `map-flow.spec.ts` 一致 |
| 16 | **`addInitScript` 必须早于 `goto`** | 包装 `window.WebSocket` 必须在页面加载前注入；放 `goto` 之后已晚，原始 WebSocket 已被使用 |
| 17 | 测试断言 `payload.x` 必须用浮点容差 | world-engine 序列化是 f32 → f64，可能有 `50.000001` 之类；断言 `Math.abs(x - 50) < 0.01` |
| 18 | **WS upgrade 的 CORS 和 HTTP CORS 是两套**| WS 升级校验走 `websocket.AcceptOptions.OriginPatterns`（`Accept` 阶段）；HTTP 走 `cors.Middleware`。两者互不替代 |

## 六、回滚

- **ws-gateway 整服务**：`git revert 65ceec7` —— 单 commit 一键回；web 端 `useEffect(startWsBridge)` 失效也不致命（断网重连退避已经够用）。但 WorldMap 删了 `setInterval`，回滚后 web 端**不会自动恢复 3s 轮询**，需要手动改 `WorldMap.tsx`
- **仅回滚 web 端**：保留 ws-gateway，把 `web/src/components/Map/WorldMap.tsx` 改回 `setInterval(fetchTiles, POLL_MS)` + 删除 `web/src/app/city/page.tsx` 的 `startWsBridge` useEffect
- **仅回滚 ws-gateway**：web 端 `startWsBridge` 调用了空实现会反复重连但不影响功能；最坏是 log 噪音
- **Docker compose**：删 `ws-gateway` service 块（无 schema 变更、无破坏性）

## 七、后续 Sprint 建议

按依赖顺序：

1. **Sprint 10**：tile/region 过滤 —— 现在 ws-gateway 全广播给所有连接；量级一大（百玩家 + 100 tile）带宽和 CPU 都会被浪费。按 tile_id 过滤：每个 Client 维护 `subscribed_tiles`，hub 只 fanout 命中 Client
2. **Sprint 10**：WS 重连后立即拉一次 —— 当前靠 WorldMap mount 时 `fetchTiles()` 兜底；显式 `onreconnect → fetchTiles()` 加在 `web/src/lib/ws-events.ts`
3. **Sprint 10**：WS 切到 `WS_VERIFY_ORIGIN=true` —— 现在 dev 默认 `InsecureSkipVerify`，任何站点都能建 WS（token 仍需合法但可被 CSRF 式滥用）。生产前切
4. **Sprint 10+**：订阅多频道 —— `REDIS_CHANNEL_NPC_DIALOGUE` 暂无 producer，但 schema 设计上 envelope `type` 字段已为多类型铺路
5. **Sprint 10+**：WorldMap 测试覆盖（vitest + jsdom）—— 现在只有 e2e 端到端验，没有组件级单测。CustomEvent 触发的 refetch 路径没单测
6. **远期**：WS 鉴权换成 subprotocol（`Sec-WebSocket-Protocol: jwt.<token>`）而不是 query string —— query string 容易落 access log；不过 dev 可接受