# Sprint 1.5 复盘

> 范围：**world-engine REST + Redis Pub/Sub + api-gateway 反向代理 + 订阅者落库**
>
> 完成时间：2026-09
>
> 提交：
> - `e9e704b` feat(sprint1.5): world-engine — RedisPub 替代 redis-rs + /healthz + /metrics
> - `2194015` feat(sprint1.5): api-gateway — Redis 订阅者 + UpsertPosition + ctx 拆分

## 一、本次交付

### world-engine (`apps/world-engine/`, Rust 1.82)

| 项 | 说明 |
|---|---|
| 内存 3×3 Tile | 默认 9 个 Tile（中心 CBD，4 邻 Residential，4 角 Suburb）+ 3 个种子 NPC |
| 玩家位置表 | `WorldGrid::upsert_position` 同步维护 `tile.player_ids` 集合（玩家切换 Tile 自动 leave/enter） |
| REST API | `/healthz` `/v1/tiles` `/v1/tiles/:id` `/v1/tiles/:id/players` `/v1/players/:id/position` `POST /v1/world/move` `/v1/_metrics` |
| Redis publisher | **手写 RESP**（`src/redis_pub.rs`），不用 `redis-rs` —— 后者 winapi 依赖链在 Win 上触发栈溢出 |
| 连接复用 | 单 `BufWriter<TcpStream>` + `tokio::sync::Mutex`，lazy connect，断线自愈 |
| 健康探针 | `RedisPub::ping()` 独立短连接发 `PING` 读 `+PONG`（1s 超时） |
| 原子计数器 | `RedisStats { messages_published, connect_errors, write_errors, flush_errors, ping_success, ping_failure }` —— lock-free `AtomicU64` |
| 单测 | 19 通过 / 1 ignored（`test_ping_live_redis` 需本地 Redis） |

### api-gateway (`apps/api-gateway/`, Go 1.23 + Gin)

| 项 | 说明 |
|---|---|
| 反向代理 | `/v1/tiles` `/v1/tiles/:id` `POST /v1/world/move` 透传到 world-engine |
| 鉴权链 | Recovery → TraceID → Logging → RateLimit → AntiScrap → Auth |
| Redis 订阅者 | `internal/subscriber/player_moved.go`：go-redis Subscribe `aicity:player:moved` → JSON 解码 → `PlayerStore.UpsertPosition`；自动重连退避 1s→5s |
| 数据落库 | `PlayerStore.UpsertPosition`：`INSERT ... ON CONFLICT (player_id) DO UPDATE` 覆盖式写入 `player_position` |
| 上下文拆分 | `bootCtx`（10s PG init）vs `appCtx`（signal-canceled）—— **修了一个会导致订阅者 10s 后静默退出的 ctx 泄漏 bug** |
| Shutdown 顺序 | SIGINT → `appCancel()` 让 subscriber 先退出 → `srv.Shutdown(30s)` graceful |

### 数据流（端到端）

```
client ──POST /v1/world/move──> api-gateway ──proxy──> world-engine
                                                          │
                                                          ├─ update WorldGrid (内存)
                                                          └─ tokio::spawn publish
                                                                 │
                                                                 ▼
                                                       Redis aicity:player:moved
                                                                 │
                                                                 ▼
                                          api-gateway subscriber (go-redis)
                                                                 │
                                                                 ▼
                                                 PG player_position (UpsertPosition)
```

## 二、验证证据（已通过）

### 1. 编译 + 单测

```bash
# world-engine
cd apps/world-engine && cargo test --lib
# → 19 passed; 0 failed; 1 ignored

# api-gateway
cd apps/api-gateway && go build ./...
# → clean
```

### 2. 端到端冒烟

```bash
# 起 PG
docker run -d --name aicity-pg -e POSTGRES_USER=aicity -e POSTGRES_PASSWORD=aicity_dev \
  -e POSTGRES_DB=aicity -p 5432:5432 postgres:15-alpine
docker exec -i aicity-pg psql -U aicity -d aicity < packages/proto/pg-schema.sql

# 起 Redis（本机已装 7.x）
redis-server   # 或 docker run -d -p 6379:6379 redis:7-alpine

# 起 world-engine（注意 127.0.0.1，Win 上 localhost DNS 偶尔 >2s）
REDIS_URL=redis://127.0.0.1:6379/0 REDIS_CHANNEL_MOVED=aicity:player:moved \
  ./target/debug/world-engine.exe

# 起 api-gateway
DATABASE_URL=postgresql://aicity:aicity_dev@localhost:5432/aicity \
REDIS_URL=redis://127.0.0.1:6379/0 WORLD_ENGINE_URL=http://127.0.0.1:50052 \
JWT_SECRET=dev-secret-change-me API_GATEWAY_PORT=8088 \
  ./bin/api-gateway.exe

# 1) 登录拿 JWT
TOKEN=$(curl -s -X POST http://localhost:8088/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"demo","password":"demo123"}' \
  | python -c "import sys,json;print(json.load(sys.stdin)['token'])")

# 2) 移动玩家
curl -X POST http://localhost:8088/v1/world/move \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"player_id":"ebf2824b-1671-401c-8c0a-30675090336b",
       "from_tile_id":"tile_0_0","to_tile_id":"tile_1_0",
       "x":150.0,"y":50.0}'

# 3) 验证 PG
docker exec aicity-pg psql -U aicity -d aicity \
  -c "SELECT player_id, tile_id, x, y, updated_at FROM player_position;"
# → ebf2824b-... | tile_1_0 | 150 | 50 | <timestamp>
```

### 3. /healthz 三态

| 场景 | 响应 |
|---|---|
| Redis 可达 | `{"status":"ok","redis":"ok","redis_stats":{...},"tiles":9,"players_tracked":1}` |
| 无 `REDIS_URL` | `{"status":"ok","redis":"not_configured",...}` |
| Redis 不可达 | `{"status":"degraded","redis":"unreachable","redis_stats":{...,"ping_failure":N},...}` |

### 4. /v1/_metrics

```json
{
  "redis": {
    "messages_published": 2,
    "connect_errors": 0,
    "write_errors": 0,
    "flush_errors": 0,
    "ping_success": 2,
    "ping_failure": 0
  },
  "tiles": 9,
  "players_tracked": 1,
  "channel_moved": "aicity:player:moved"
}
```

## 三、本次踩到的坑（避免重复踩）

| # | 问题 | 解决 |
|---|---|---|
| 1 | `redis-rs` winapi 依赖链在 Win 上触发栈溢出 | 手写 `redis_pub.rs`，仅支持 PUBLISH/PING |
| 2 | `RedisPub::new(url)` 直接把 `redis://host:port/db` 给 `TcpStream::connect` → connect 失败 | 加 `parse_redis_addr` 提取 `host:port` |
| 3 | `localhost` DNS 解析在 Win 上偶尔 >2s，2s connect timeout 太紧 | dev 用 `127.0.0.1`（生产无所谓） |
| 4 | `api-gateway main.go` 把 PG init 的 10s ctx 传给 subscriber → 10s 后 ctx 取消，subscriber 静默退出，不报错 | 拆 `bootCtx` + `appCtx`，subscriber 用 `appCtx` |
| 5 | `PlayerStore.FindByUsername` 扫到 NULL `avatar_url` → `cannot scan NULL into *string` | `SELECT COALESCE(avatar_url, '') AS avatar_url` |
| 6 | Cargo release 链缺 `dlltool.exe` | 本地用 `cargo build`（debug） |
| 7 | 50052 端口 TIME_WAIT（os error 10048） | 等 10-15s 再 restart；或 `SO_REUSEADDR`（暂未启用） |

## 四、未做（明确留给 Sprint 2+）

### Sprint 2 候选

- [ ] **PG `tile` 表 + sqlx 加载**：当前 `default_world()` 是 hard-coded 3×3，Sprint 2 从 PG `tile` 读取，初始化与运行时配置解耦
- [ ] **grpc server (tonic)**：world-engine 的 `main.rs` 里还是 TODO；Sprint 2 接入 proto
- [ ] **Redis 订阅端的反向通知**：当前 world-engine 单向 publish 给 gateway；Sprint 2 需要 world-engine 订阅自己实例间的同步消息（多实例部署）
- [ ] **Prometheus 接入**：api-gateway `go.mod` 已有 `prometheus/client_golang`，world-engine 暂无；先定 metrics 命名（`worldengine_redis_publish_total` 之类），再写到 `/metrics`（Prometheus 格式）
- [ ] **HealthZ → readiness/liveness 分离**：当前 `/healthz` 既检查进程存活也检查 Redis 连通；K8s 场景应拆 `liveness`（存活）和 `readiness`（可接流量）

### 待优化（非阻塞）

- [ ] `redis_pub.rs::publish` 失败时把 `RedisStats` 上报 OTLP，便于看时间分布
- [ ] `RedisPub::ping` 每次建短连接 → 高频探针下考虑复用 + PING-only connection
- [ ] 端口冲突方案：`SO_REUSEADDR`（Linux/Win）+ K8s 用 `service.kubernetes.io/port` 自动避让
- [ ] world-engine `bin/` 目录里 `api-gateway` 误入（gitignore 已忽略，但路径错位）—— Makefile 重新组织构建

## 五、变更清单

```
ai-city/apps/api-gateway/cmd/main.go                            (M)
ai-city/apps/api-gateway/internal/handlers/auth.go              (M)
ai-city/apps/api-gateway/internal/router/router.go              (M)
ai-city/apps/api-gateway/internal/store/player.go               (M)  +UpsertPosition, +COALESCE
ai-city/apps/api-gateway/internal/subscriber/player_moved.go    (A)  new
ai-city/apps/world-engine/Cargo.toml                            (M)  +axum/tower
ai-city/apps/world-engine/src/lib.rs                            (M)  +pub mod redis_pub
ai-city/apps/world-engine/src/main.rs                           (M)  redis::Client → RedisPub
ai-city/apps/world-engine/src/rest.rs                           (M)  +/healthz, +/v1/_metrics
ai-city/apps/world-engine/src/world_grid.rs                     (M)  +player_count
ai-city/apps/world-engine/src/redis_pub.rs                      (A)  new
```

## 六、下一步建议

按依赖顺序：

1. **把 Sprint 1.5 推送到 origin/main**
2. **Sprint 2 规划**：`tile` PG 表 schema + sqlx 加载 + CRUD API
3. **CI 接入**：GitHub Actions → `cargo test` + `go test` + `golangci-lint` + `cargo clippy`
4. **Storybook / E2E**：Playwright 跑端到端（登录 → 移动 → 验证 DB row）作为 CI gate
