# Sprint 10 复盘

> 范围：**ws-gateway 硬化 + 缩放** —— tile 过滤基础设施、WS 重连校准、Origin 验证、多频道铺路
>
> 完成时间：<待补>
>
> 提交：<待补>

## 〇、本文档定位

Sprint 10 是 Sprint 9 "ws-gateway 实装" 的硬化与缩放轮：4 个明显的技术债点
（retro §七.1-4）一次性收口。

**重要：本 retro 在实现前定稿。** commit hash / E2E 输出 / 真实 go test 计数
留作 `<待补>` 占位。设计层面已与 code review 阶段对齐（见 §三、§五），未
列出的隐式选择都视为 Sprint 9 既有约定的延续。

## 一、本次交付

```
Sprint 9 (既有)
  world-engine ─pub─> Redis ─sub─> api-gateway   (PG 写位置)
                    └─sub─> ws-gateway ─ws─> web (实时推送)

Sprint 10 (本次)
  ws-gateway
    ├─ hub: 加 tileClients 索引 + BroadcastToTile
    │       → 9 tile × N client 时行为不变；数据结构为 Sprint 11+ 真过滤铺路
    ├─ subscriber: 多频道 Subscribe() + aicity:<type> 前缀剥离
    │       → world-engine 不动；ws-gateway 侧结构铺好，新 producer 零改动接入
    └─ cmd: 多频道 + 默认 9 tile 注入

  web
    ├─ ws.ts: hasConnectedOnce flag + onReconnect(fn) 钩子
    └─ WorldMap: 抽 debounce 工具 + 监听 aicity:ws_reconnected → 200ms refetch

  ws_smoke
    └─ case 7 (opt-in, WS_SMOKE_VERIFY_ORIGIN=true 才跑): 验 evil origin 被拒
```

### A. tile/region 过滤基础设施（hub）

**核心改动**：hub 单 goroutine 独占的数据结构上加一个反向索引 `tileClients`。
filtering 逻辑启用但默认行为不变（client 默认订阅全集 9 tile → BroadcastToTile
等价于 Broadcast），Sprint 11+ 改 Client.SubscribedTiles 注入即可启用真过滤。

| 项 | 说明 |
|---|---|
| 新字段 | `Hub.tileClients map[string]map[*Client]struct{}` —— tile → 订阅该 tile 的 client 集合 |
| Client 字段 | `SubscribedTiles []string` —— 构造时一次性注入（NewClient 加参数） |
| 新方法 | `Hub.BroadcastToTile(tileID string, msg []byte)` —— 只 fanout `tileClients[tileID]` |
| 既有 `Broadcast` | **保留**（兜底 + 无 tile 亲和的消息如 npc_dialogue 走全广播）|
| 新 channel | `broadcastTile chan tileMessage`（结构 `{tileID, msg}`）—— 与 broadcast 分离，独立 select case |
| 计数 | 复用 `broadcasts` 计数器（一次计数 = 一次 fanout 尝试；`delivered` / `evicted` 语义不变）|
| Register 维护 | 插入 `clients` 后遍历 `c.SubscribedTiles` 写入 `tileClients[t]` |
| Unregister 维护 | 从 `clients` 删除后遍历 `c.SubscribedTiles` 从 `tileClients[t]` 删除；**空 set 立即 `delete(tileClients, t)`**（防止 long-lived hub 内存泄漏）|
| 驱逐跨清理 | 慢消费者驱逐时**同时**从 `clients` 和 `tileClients[t]` 删除 —— 否则被踢客户端会从全广播 `Broadcast` 路径继续收消息 |

**subscriber.handle 路由**（关键决策）：

- envelope 解析出 `tile_id` 非空 → `BroadcastToTile(tileID, msg)`
- envelope 解析出 `tile_id` 为空（未来 npc_dialogue 等）→ `Broadcast(msg)` 兜底
- JSON 解析失败 → log warn + 跳过（不广播）

**默认订阅全集**（main.go 注入）：

```go
// 3×3 网格，与 world-engine 当前硬编码一致
defaultTiles := []string{
    "tile_-1_-1", "tile_-1_0", "tile_-1_1",
    "tile_0_-1",  "tile_0_0",  "tile_0_1",
    "tile_1_-1",  "tile_1_0",  "tile_1_1",
}
```

→ env `WS_DEFAULT_TILES`（CSV，可覆盖；为将来 10×10 网格铺路）

**Broadcaster interface 扩展**：

```go
type Broadcaster interface {
    Broadcast(msg []byte)
    BroadcastToTile(tileID string, msg []byte)   // 新增
}
```

`fakeBroadcaster`（subscriber_test.go）补一个 `BroadcastToTile` 计数实现，与 `Broadcast` 并列。

### B. WS reconnect → fetchTiles

**核心改动**：web 端 WS 重连后立刻拉一次 tile 数据，校准离线期间错过的世界状态。
当前依赖 WorldMap mount 时 `fetchTiles()` 兜底；reconnect 路径在重连瞬间触发，
比 mount 触发更精准（mount 只在组件挂载时跑，重连不会重新挂载）。

| 项 | 说明 |
|---|---|
| `WSClient` 字段 | `hasConnectedOnce bool` —— 默认 false，首次 onopen 翻 true |
| `WSClient.onReconnect(fn)` | 返回 unsubscribe 函数；onopen 触发时若 `hasConnectedOnce` 已为 true 则回调所有 listener |
| `ws-events.ts` | 新常量 `WS_RECONNECTED_EVENT = 'aicity:ws_reconnected'`；`startWsBridge()` 挂 `ws.onReconnect(() => dispatchEvent(WS_RECONNECTED_EVENT))` |
| `WorldMap.tsx` | 抽 file-local `debounceRefetch(fn, 200ms)`；两个 listener（PLAYER_MOVED + WS_RECONNECTED）共用一个 timer —— 巧合的 reconnect + player_moved 折叠成一次 fetch |
| 行为 | reconnect 瞬间立刻校准；离线期间 fetch 失败按 WorldMap 既有错误路径处理（不向用户暴露 fatal）|

**为什么不抽共享 debounce lib**：Sprint 10 只有 WorldMap 一个消费者。KISS —— 第二次出现时再抽。

### C. WS_VERIFY_ORIGIN 验证测试

**核心改动**：ws_smoke 加 case 7（opt-in）+ docker-compose 注释说明 prod 验收步骤。

| 项 | 说明 |
|---|---|
| 触发条件 | 仅 `WS_SMOKE_VERIFY_ORIGIN=true` 时跑（默认跳过，dev 不需要）|
| 用例 | 独立 dial，Origin header = `http://evil.example`；期望 dial 失败（nhooyr 在收到非 101 响应时返回 error）|
| 位置 | 既有 6 步**之后**（case 7 失败不阻塞 1-6 报告）|
| 兜底 | CORS middleware（`internal/cors/`）用 `cors.HostPatterns(allowed)` 剥 scheme 后匹配 `evil.example` 不在 allowlist → 403 |
| docker-compose | 在 ws-gateway service env 块加注释说明：取消注释 `WS_VERIFY_ORIGIN: 'true'` → `docker compose up -d --force-recreate ws-gateway` → `WS_SMOKE_VERIFY_ORIGIN=true docker compose exec -T ws-gateway /app/ws_smoke` |

### D. 多频道订阅（结构铺路，不接 producer）

**核心改动**：ws-gateway 改为可同时订阅多 Redis 频道；频道名 → envelope `type` 用前缀剥离约定。
world-engine 目前**只** publish `aicity:player:moved`，本 sprint **不接** 任何新 producer。

**频道 → type 约定**（注释强调）：

```
频道名 → envelope type 用前缀剥离：
  aicity:player:moved  →  player_moved
  aicity:npc_dialogue  →  npc_dialogue
新频道只需遵守 `aicity:<type>` 命名。
```

| 项 | 说明 |
|---|---|
| Config 字段 | `Channels []string` 新增；`ChannelMoved string` 保留（向后兼容）|
| 加载逻辑 | `REDIS_CHANNELS`（CSV）非空时 → `cfg.Channels`；否则 `cfg.Channels = [cfg.ChannelMoved]`（fallback 兜底）|
| Subscriber 函数 | 新增 `Subscribe(ctx, rdb, channels []string, b, logger)`：单 conn 订阅多频道（`rdb.Subscribe(ctx, channels...)` + `pubsub.Channel()`）|
| `PlayerMoved` 单频道版 | **保留**（向后兼容；现有测试引用）|
| envelope 构造 | 重构 `handle` 接受 `(channel, payload, ...)`；剥前缀拿 type 后构信封 + 调 dispatch（tile 路由 + trace_id / ts_ms enrichment 都在这一步）|
| trace_id / ts_ms enrichment | 服务端统一添加（不依赖 producer）；envelope 字段是 single source of truth |
| 兜底 | `len(cfg.Channels) == 0` 在 `main.go` 显式 `logger.Fatal` —— 防 `REDIS_CHANNELS=""` 静默订阅零频道 |
| 单/多共享 | `PlayerMoved` 改薄适配器（固定 channel 调 `handle`）；单/多走同一条 envelope 构造路径 |
| main.go | `wsredis.Subscribe(appCtx, rdb, cfg.Channels, h, logger)` 替原 `PlayerMoved` 单频道调用 |

### E. 测试策略

**Always-run 单元测**（新增）：

| 测试 | 覆盖 |
|---|---|
| `TestHub_BroadcastToTile` 4 case | (1) A 订阅 [tile_0_0] + B 订阅 [tile_1_0]，BroadcastToTile("tile_0_0") → A 收 / B 不收；(2) 不订阅的 client 不收；(3) Unregister 后从 tileClients 索引清理，再 BroadcastToTile 不投递；(4) Client 订阅多 tile，任一 BroadcastToTile 都收到 |
| `TestHub_BroadcastToTile_NoSubscribers` | tile 上零订阅时 BroadcastToTile 不 panic，counter 不变 |
| `TestHub_BroadcastToTile_AfterShutdown` | ctx cancel 后 BroadcastToTile 静默返回（与既有 TestHub_ShutdownClosesClients 对称）|
| `TestHub_FilterIndexConsistentWithClientsMap` | race detector ×3：边 register/unregister 边 broadcast，tileClients 不漏不重 |
| `TestHandle_RoutesByTile` | `handle()` 解析 tile_id → 走 BroadcastToTile；未解析 → 走 Broadcast；JSON 错 → 跳过 |
| `TestHandle_EnvelopeTraceIDAndTSMS` | 每个 channel 的 envelope 都含新生成的 trace_id + ts_ms |
| `TestEnvelop_ChannelTypeCollisions` | `aicity:foo` → `foo`；`aicity:foo:bar` → `foo:bar`；两者不冲突（防未来 contributor 改剥前缀逻辑引入碰撞）|
| `TestSubscribe_ChannelToTypeStrip` | channel 字符串 → envelope type 的纯函数映射（无 I/O）|
| `fakeBroadcaster` 扩展 | `Broadcast` / `BroadcastToTile` 各一个计数器；保留向后兼容 |

**Env-gated 集成测**：

- `TestSubscribe_Integration` —— 真 Redis 订阅两频道 + publish 不同 channel payload + 验 envelope type 各自不同
- 复用 `WS_TEST_REDIS_URL` env 模式（与 `subscriber_test.go` 既有约定一致）

**E2E**：

- `ws_smoke/main.go` 加 case 7（条件跑）—— 详见 §C
- 既有 Playwright 不动（reconnect 模拟需 200ms 内断 WS，E2E 复杂，risk/reward 不划算；手动验证）

## 二、E2E 输出

### 单元测（实施后回填）

```bash
$ export PATH="/d/aicode/w64devkit-1.23.0/w64devkit/bin:/c/Users/wma19/.cargo/bin:$PATH"
$ cd ai-city/apps/ws-gateway && go test ./... -count=1 -race
?       github.com/aicity/ws-gateway/cmd    [no test files]
?       github.com/aicity/ws-gateway/cmd/ws_smoke    [no test files]
ok      github.com/aicity/ws-gateway/internal/auth    <待补>
ok      github.com/aicity/ws-gateway/internal/cors    <待补>
ok      github.com/aicity/ws-gateway/internal/hub     <待补>
ok      github.com/aicity/ws-gateway/internal/protocol    <待补>
ok      github.com/aicity/ws-gateway/internal/redis   <待补>     # 集成测 WS_TEST_REDIS_URL 未设 → SKIP
PASS
```

### 7 容器健康

```bash
$ MSYS_NO_PATHCONV=1 docker compose up -d --build
$ MSYS_NO_PATHCONV=1 docker compose ps
NAME                    IMAGE                 STATUS                    PORTS
aitown-a2a-gateway-1    aitown-a2a-gateway    Up (healthy)
aitown-api-gateway-1    aitown-api-gateway    Up (healthy)
aitown-postgres-1       postgres:16-alpine    Up (healthy)
aitown-redis-1          redis:7-alpine        Up (healthy)
aitown-web-1            aitown-web            Up
aitown-world-engine-1   aitown-world-engine   Up (healthy)
aitown-ws-gateway-1     aitown-ws-gateway     Up (healthy)              ← Sprint 10 新镜像
```

### ws_smoke 6/6（行为不变）

```bash
$ MSYS_NO_PATHCONV=1 docker compose exec -T ws-gateway /app/ws_smoke
[OK]   1/6 login user=demo player_id=...
[OK]   2/6 ws connected ws://127.0.0.1:8082/ws
[OK]   3/6 ws without token rejected
[OK]   4/6 move accepted tile=tile_0_0 pos=(50.0,50.0)
[OK]   5/6 player_moved received trace_id=... ts_ms=...
[OK]   6/6 payload player_id=... tile_id=tile_0_0 pos=(50.0,50.0)
[OK] all 6 ws_smoke checks passed (api=http://api-gateway:8080 ws=ws://127.0.0.1:8082/ws)
```

### ws-gateway 启动日志确认订阅频道正确

```bash
$ MSYS_NO_PATHCONV=1 docker compose logs --tail=20 ws-gateway | grep -E "subscriber (starting|ready)|tile_filter"
ws-gateway-1  | {"level":"info","msg":"subscriber starting","channels":["aicity:player:moved"],"client_count":0}
ws-gateway-1  | {"level":"info","msg":"subscriber ready","channels":["aicity:player:moved"]}
ws-gateway-1  | {"level":"info","msg":"tile_filter default_subscriptions","tiles":9,"source":"default"}
```

### 多频道手动验证（环境变量覆盖）

```bash
$ MSYS_NO_PATHCONV=1 docker compose stop ws-gateway
$ MSYS_NO_PATHCONV=1 REDIS_CHANNELS="aicity:player:moved,aicity:test:foo:bar" \
    docker compose up -d ws-gateway
$ MSYS_NO_PATHCONV=1 docker compose logs --tail=10 ws-gateway | grep "subscriber starting"
ws-gateway-1  | {"level":"info","msg":"subscriber starting","channels":["aicity:player:moved","aicity:test:foo:bar"]}
```

### 既有 Playwright E2E 不动

```bash
$ cd ai-city/web && pnpm exec playwright test
Running 3 tests using 1 worker
  ✓  1 [chromium] › e2e\map-flow.spec.ts:11:5 › login → /city → 9 tiles visible → click move → position changed
  ✓  2 [chromium] › e2e\map-flow.spec.ts:140:5 › login failure shows error message
  ✓  3 [chromium] › e2e\ws-push.spec.ts:42:5 › 进 /city 后 WS 连上，移动触发 player_moved 推送
3 passed
```

## 三、关键决策

| # | 决策 | 理由 |
|---|---|---|
| 1 | tileClients 用 `map[string]map[*Client]struct{}` 反向索引 | 单 goroutine 维护，无需锁；O(1) lookup；set 语义避免重复 |
| 2 | Client.SubscribedTiles 构造时一次性注入 | 单 goroutine 索引维护简单；运行时变更协议复杂度不值得；Sprint 11+ 再加 subscribe 协议 |
| 3 | 默认订阅全集 9 tile | 行为不变（BroadcastToTile 与 Broadcast 等价）；数据结构落地；Sprint 11+ 改注入即可 |
| 4 | `BroadcastToTile` + `Broadcast` 并存 | `player_moved` 走 tile 路径；未来 `npc_dialogue` 等无 tile 亲和走全广播；语义清晰 |
| 5 | `BroadcastToTile` 走独立 `broadcastTile chan tileMessage` | 与 broadcast 分离；独立 select case；为未来 per-tile 计数器留口 |
| 6 | tileClients 空 set 立即删除 | long-lived hub 内存不随 client churn 增长 |
| 7 | 慢消费者驱逐时同时清 `clients` + `tileClients[t]` | 否则被踢客户端从全广播 `Broadcast` 路径继续收消息 |
| 8 | 复用 `broadcasts` 计数器（含 `BroadcastToTile`）| 一个数 = 一次 fanout 尝试；与既有 dashboard 契约一致 |
| 9 | 多频道 type 用前缀剥离（`aicity:<type>`）| 零配置；命名即文档；新频道零服务端改动 |
| 10 | `PlayerMoved` 单频道版保留 | 既有测试引用；向后兼容；新 `Subscribe` 替代 main.go 入口 |
| 11 | `handle` 重构接受 `(channel, payload, ...)` | 单/多共享 envelope 构造；单 `PlayerMoved` 变薄适配器 |
| 12 | 服务端统一 enrich `trace_id` + `ts_ms`（不依赖 producer）| single source of truth；防 producer 漂移 |
| 13 | 保留 `REDIS_CHANNEL_MOVED` 单数 env | compose 已用；`REDIS_CHANNELS` 非空才覆盖 |
| 14 | `len(cfg.Channels) == 0` 显式 `logger.Fatal` | 防 `REDIS_CHANNELS=""` 静默订阅零频道 |
| 15 | `hasConnectedOnce` flag 区分首次/重连 | onopen 二次起算；onclose→onopen 周期里只第二次起算 reconnect |
| 16 | reconnect listener 共用 PLAYER_MOVED_EVENT debounce | 两类事件触发逻辑相同；巧合的 reconnect + player_moved 折叠一次 fetch |
| 17 | debounce 工具 file-local 在 WorldMap（不抽 lib）| Sprint 10 单消费者；KISS；第二次出现时再抽 |
| 18 | ws_smoke case 7 跑在 6 步之后 | 失败不阻塞 1-6 报告；opt-in 不影响默认行为 |
| 19 | case 7 用 nhooyr `DialOptions.HTTPHeader` 设 Origin | 跟 CORS middleware 走同一个 accept 路径；nhooyr 在非 101 响应时返回 error |
| 20 | Compose 注释（不开 profile）| 仅一 flag 翻转；profile 需复制整 service 块，diff 更大 |
| 21 | 不写 Playwright reconnect 测试 | 200ms 内断 WS 的模拟复杂；risk/reward 不划算；手动 devtools 验证 |
| 22 | 不接任何新 producer | 频道结构铺好；接 producer 是独立 sprint（建议 Sprint 11+）|

## 四、变更清单

### 新增（<待补>）

ws-gateway：
- 测试文件新增：`TestHub_BroadcastToTile` 4 case + 2 边缘 case + race detector ×3
- 测试文件新增：`TestHandle_RoutesByTile` + `TestHandle_EnvelopeTraceIDAndTSMS` + `TestEnvelop_ChannelTypeCollisions`
- 测试文件新增：`TestSubscribe_ChannelToTypeStrip`（纯函数）+ `TestSubscribe_Integration`（env-gated）
- `fakeBroadcaster` 扩展：`BroadcastToTile` 计数器

web：
- `web/src/lib/ws.ts` 的 `WSClient` 加 `hasConnectedOnce` 字段 + `onReconnect` 公开方法
- `web/src/lib/ws-events.ts` 加 `WS_RECONNECTED_EVENT` 常量 + startWsBridge 多挂 onReconnect
- `web/src/components/Map/WorldMap.tsx` 加 file-local `debounceRefetch` + `WS_RECONNECTED_EVENT` listener

docs：
- `docs/SPRINT-10.md` —— 本文件

### 修改（<待补>）

- `apps/ws-gateway/internal/hub/hub.go` —— 加 `tileClients` 字段 + `BroadcastToTile` 方法 + `broadcastTile` chan + 维护索引 + 空 set 删除
- `apps/ws-gateway/internal/hub/client.go` —— `NewClient` 加 `tiles []string` 参数 + `SubscribedTiles` 字段
- `apps/ws-gateway/internal/redis/subscriber.go` —— 加 `Subscribe` 多频道 + `handleMulti` 路由 + `handle` 重构接受 channel
- `apps/ws-gateway/internal/config/config.go` —— 加 `Channels []string` + `DefaultTiles []string` 字段 + 加载逻辑
- `apps/ws-gateway/cmd/main.go` —— 用 `cfg.Channels` + `cfg.DefaultTiles` + `wsredis.Subscribe` 替 `PlayerMoved` + 显式空 channels 校验
- `apps/ws-gateway/cmd/ws_smoke/main.go` —— 加 case 7（opt-in，env gate）
- `apps/ws-gateway/internal/redis/subscriber_test.go` —— `fakeBroadcaster` 加 `BroadcastToTile` 计数 + 新测试
- `apps/ws-gateway/internal/hub/hub_test.go` —— 新测试 + 调整 `NewClient` 调用点
- `docker-compose.yml` —— ws-gateway env 注释说明 `WS_VERIFY_ORIGIN` 切 prod 步骤
- `web/src/lib/ws.ts` —— `hasConnectedOnce` + `onReconnect` 钩子
- `web/src/lib/ws-events.ts` —— `WS_RECONNECTED_EVENT` + startWsBridge 多挂
- `web/src/components/Map/WorldMap.tsx` —— 抽 debounce 工具 + 监听 reconnect 事件

### 删除（0）

无 —— 完全是增量。`Broadcast` 旧路径保留；`PlayerMoved` 单频道函数保留；既有 6 步 smoke 不动。

## 五、关键陷阱

| # | 陷阱 | 缓解 |
|---|---|---|
| 1 | **`BroadcastToTile` 走独立 channel 而不是改 `broadcast` 的 msg 结构** | plan 原表述有歧义；改：新增 `broadcastTile chan tileMessage` + 独立 select case，保留 `broadcast` 不动 |
| 2 | **tileClients 空 set 不删会内存泄漏** | long-lived hub + ephemeral client → 每 client × 每 tile 留一个空 map；Unregister 路径里 `if len(set)==0 { delete(tileClients, t) }` 守住 |
| 3 | **慢消费者驱逐跨索引清理** | 驱逐路径必须同时 `delete(h.clients, c)` + `delete(h.tileClients[t], c)`；否则被踢客户端从全广播 `Broadcast` 路径继续收消息 |
| 4 | **`NewClient` 签名变化级联到所有调用点** | 4 参 → 5 参；grep 所有 `NewClient(` 调用点（main.go + hub_test.go + client_test.go）一起改 |
| 5 | **`handle` 重构后单/多频道走同一条 envelope 构造路径** | 不重复实现；`PlayerMoved` 变薄适配器（固定 channel 调 `handle`）；`fakeBroadcaster` 同时实现 `Broadcast` + `BroadcastToTile` |
| 6 | **envelope `trace_id` / `ts_ms` enrich 必须在服务端** | 不依赖 producer；新 channel 立刻获得观测能力；防止 producer 漂移 |
| 7 | **`rdb.Subscribe(ctx, channels...)` 在 `len(channels)==0` 时 panic/error** | config loader 默认 fallback 到 `[ChannelMoved]`；main.go 仍加显式 `logger.Fatal` 兜底（防 `REDIS_CHANNELS=""` 边界）|
| 8 | **nhooyr `websocket.Dial` 必须传 context** | ws_smoke case 7 用 3s timeout ctx（与既有 case 一致）|
| 9 | **CORS middleware 跟 WS upgrade 是两套** | 既有 §一-A 决策沿用：WS 走 `cors.HostPatterns`；HTTP 走 `cors.Middleware`。case 7 只验 WS upgrade |
| 10 | **hasConnectedOnce 必须默认 false** | 首次 onopen 翻 true 前**不能**触发 onReconnect；flag 误初始化为 true 会让 mount 期间误触发 fetch |
| 11 | **ws.ts `onReconnect` 多次挂载的 listener 集合** | 用 `Set<() => void>`；unsubscribe 返回 `Set.delete` 闭包（与 `onMessage` 同款）|
| 12 | **WorldMap 抽的 debounce 工具放 file-local 不抽 lib** | 第二次出现时再抽；KISS；防 premature abstraction |
| 13 | **ws_smoke case 7 跑在 6 步之后** | 失败不阻塞 1-6 报告；opt-in env 不影响默认 6/6 |
| 14 | **MSYS_NO_PATHCONV=1 docker compose exec** | 不设的话 `/app/ws_smoke` 被 MSYS 转成 `C:/Program Files/Git/app/ws_smoke`（Sprint 9 已知坑）|
| 15 | **nhooyr `DialOptions.HTTPHeader` 用 `http.Header{"Origin": [...]}`** | 标头名是字符串字面量；用 `http.Header{}.Set` 也行但要求 URL-canonical；显式构造最稳 |
| 16 | **`aicity:foo` 与 `aicity:foo:bar` 剥前缀后 type 不同** | `foo` ≠ `foo:bar`；不冲突；`TestEnvelop_ChannelTypeCollisions` 守住 |
| 17 | **go-redis 重复频道在 CSV 里自动去重** | `rdb.Subscribe` 内部用 set；CSV 多余项零影响；config 注释说明 |
| 18 | **`fakeBroadcaster` 扩展 break 既有 `TestHandle_*`** | 同时改 fakeBroadcaster 加 `BroadcastToTile` 计数；既有测试用 `Broadcast` 断言的仍可继续（新增 `BroadcastToTile` 计数为独立维度）|

## 六、回滚

- **整套 Sprint 10**：单 commit revert 即可（无 schema 变更；无破坏性数据迁移；频道类型都是 envelope 字符串）
- **仅回滚 tile 过滤**：`tileClients` + `BroadcastToTile` 路径全删，subscriber.handle 改回只调 `Broadcast(msg)`（行为完全等价于 Sprint 9，因为默认订阅全集）
- **仅回滚多频道**：删除 `Subscribe` 多频道版 + config `Channels` 字段 + `REDIS_CHANNELS` env；main.go 改回调 `PlayerMoved` 单频道
- **仅回滚 WS reconnect 校准**：`web/src/lib/ws.ts` 的 `onReconnect` hook + `WS_RECONNECTED_EVENT` + WorldMap 监听全部删除（世界仍 mount 时 fetch 兜底）
- **仅回滚 ws_smoke case 7**：删除 `WS_SMOKE_VERIFY_ORIGIN` env gate 块（不影响 1-6）

## 七、后续 Sprint 建议

按依赖顺序：

1. **Sprint 11+**：Client→Server subscribe_tile / subscribe_region 协议 —— player position 上行 + 服务端按 viewport 过滤；本次 Client 仍 server 注入全集
2. **Sprint 11+**：真实 `npc_dialogue` producer 接入（world-engine 或 a2a-gateway）+ a2a-gateway 落 inbox
3. **Sprint 11+**：玩家位置细化 —— `/v1/tiles` 补 `player (x, y)` 字段，配合 Sprint 9 WS 推送
4. **Sprint 11+**：NPC / 建筑 click → chat 交互（WorldMap onSvgClick 已留好 polygon/circle 早返回分支）
5. **Sprint 11+**：tile 边界平滑 —— LOD + 视口裁剪；世界扩到 10×10+ 时必需（本次的 `tileClients` 反向索引为这一波铺路）
6. **远期**：WS 鉴权改 subprotocol（`Sec-WebSocket-Protocol: jwt.<token>`）—— query string 易落 access log
7. **远期**：WorldMap 组件级单测（vitest + jsdom）—— 现在只有 e2e 端到端验，reconnect → debounce → refetch 路径无单测
8. **远期**：把 Sprint 10 多频道结构反向应用到 api-gateway 的 `subscriber/player_moved.go`（目前仍单频道，量级一大同样需要去耦）

---

## 附录 A：设计 review 中已锁定的待办

实施前必须确认的开放问题（来自 plan + design review）：

- [ ] `BroadcastToTile` 走独立 `broadcastTile chan tileMessage`（非 broadcast 包装）
- [ ] Unregister 时 tileClients 空 set 立即删除
- [ ] 驱逐时同时清 `clients` + `tileClients[t]`
- [ ] `handle` 重构接受 `(channel, payload, ...)`；`PlayerMoved` 变薄适配器
- [ ] 服务端统一 enrich `trace_id` + `ts_ms`
- [ ] `len(cfg.Channels) == 0` main.go 显式 `logger.Fatal`
- [ ] ws_smoke case 7 跑在 6 步之后
- [ ] WorldMap 的 debounce 工具 file-local，不抽 lib
- [ ] `TestEnvelop_ChannelTypeCollisions` 新增（防未来剥前缀逻辑漂移）
- [ ] `TestHub_BroadcastToTile_NoSubscribers` + `TestHub_BroadcastToTile_AfterShutdown` 新增
- [ ] `TestHandle_EnvelopeTraceIDAndTSMS` 新增
- [ ] `fakeBroadcaster` 同时实现 `Broadcast` + `BroadcastToTile` 计数
- [ ] grep 所有 `NewClient(` 调用点 + 同步改签名

## 附录 B：原 plan 决策表 §F 的逐项确认

| plan §F 决策 | 确认 / 调整 |
|---|---|
| 默认订阅全集 9 tile | 确认 |
| Client.SubscribedTiles 构造时确定 | 确认 |
| BroadcastToTile + Broadcast 并存 | 确认 |
| 多频道 type 用前缀剥离 | 确认 |
| 保留 REDIS_CHANNEL_MOVED 单数 env | 确认 |
| ws.ts onReconnect 用 hasConnectedOnce flag | 确认 |
| reconnect listener 共用 PLAYER_MOVED_EVENT debounce | 确认（file-local debounce）|
| WS_SMOKE_VERIFY_ORIGIN opt-in env | 确认 |
| Compose 注释而非新 profile | 确认 |
| 不写 Playwright reconnect 测试 | 确认 |
