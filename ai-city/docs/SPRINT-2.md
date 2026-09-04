# Sprint 2 复盘

> 范围：**PG `tile` 表 + sqlx-like 手写客户端 + Prometheus /metrics + liveness/readiness 拆分**
>
> 完成时间：2026-09-04
>
> 提交：
> - `feat(sprint2): PG tile 表 + 手写 PG 客户端 + sqlx 替代方案（rust-toolchain 不升）+ Prometheus + HealthZ 拆分`

## 一、本次交付

### world-engine（Rust）

| 项 | 说明 |
|---|---|
| PG schema | `pg-schema.sql` 新增 `tile` 表 + 9 行种子数据（3×3 + 5 buildings + 3 NPC） + `schema_version` 升到 `2.3.0` |
| PG 客户端 | **手写** `src/pg_client.rs`（不引入 sqlx）；仅支持 Simple Query（Q 消息）+ trust 鉴权 |
| Tile 加载 | `src/tile_loader.rs::load_tiles(conn)` → `HashMap<String, Tile>`，把 SQL 各列 `::text` 后 serde_json 解析 `buildings` / `npc_ids` |
| 启动路径 | `main.rs`：若 `DATABASE_URL` 已设，调 `pg_client::connect` + `load_tiles` → `WorldGrid::with_tiles`；失败 fallback `default_world()` + `warn!` |
| Prometheus | `src/metrics.rs::render` → text format；**6 counter + 2 gauge** 命名遵循 `_total` 后缀 / gauge 无后缀 |
| 路由 | 新增 `/metrics`（Prometheus text），`/healthz` 改 liveness（200 always），新增 `/readyz`（Redis + tiles + pg → 200，否则 503 + reasons） |
| 单测 | **24 通过 / 0 failed / 3 ignored**（Sprint 1.5 是 19 通过 / 1 ignored；新增 PG parse、LodLevel 转换、metrics render 等 5 个 case） |
| 集成测 | `test_query_simple_live_pg` + `test_load_tiles_roundtrip` `#[ignore]` 需本机 aicity-pg，已通过 |

### api-gateway（Go）

| 项 | 说明 |
|---|---|
| 路由 | `router.go` 新增 `GET /metrics`（`gin.WrapH(promhttp.Handler())`） |
| 默认 collectors | `main.go` 构造 `collectors.NewGoCollector()` + `NewProcessCollector(...)`（构造时自动注册到 default registry，**不要**再 `MustRegister`） |
| 单测 | `go build ./...` clean |

### 数据流（未变，Sprint 1.5 已闭环）

```
client ──POST /v1/world/move──> api-gateway ──proxy──> world-engine
                                                          │
                                                          ├─ update WorldGrid (PG tile + 内存 player)
                                                          └─ tokio::spawn publish
                                                                 │
                                                                 ▼
                                                       Redis aicity:player:moved
                                                                 │
                                                                 ▼
                                          api-gateway subscriber (go-redis)
                                                                 │
                                                                 ▼
                                                 PG player_position (UPSERT)
```

## 二、验证证据（已通过）

### 1. world-engine /metrics（Prometheus text format）

```
# HELP worldengine_redis_publish_total Number of successful Redis PUBLISH messages
# TYPE worldengine_redis_publish_total counter
worldengine_redis_publish_total 1
# HELP worldengine_redis_ping_success_total Number of successful Redis PING (got +PONG)
# TYPE worldengine_redis_ping_success_total counter
worldengine_redis_ping_success_total 2
# HELP worldengine_tiles_loaded Number of tiles currently loaded in WorldGrid
# TYPE worldengine_tiles_loaded gauge
worldengine_tiles_loaded 9
# HELP worldengine_players_tracked Number of players currently tracked in WorldGrid
# TYPE worldengine_players_tracked gauge
worldengine_players_tracked 1
```

### 2. liveness / readiness 拆分

| 场景 | `/healthz` | `/readyz` |
|---|---|---|
| Redis ok + tiles 9 + pg connected | 200 `{"status":"alive"}` | 200 `{"status":"ready","redis":"ok","pg_connected":true,"tiles":9,...}` |
| Redis 杀停 | 200 `{"status":"alive"}` | **503** `{"status":"not_ready","reasons":["redis_unreachable"],...}` |
| Redis 重启后 | 200 | 200 自动恢复 |

### 3. api-gateway /metrics

```
# HELP go_gc_duration_seconds A summary of the wall-time pause ...
# HELP go_goroutines Number of goroutines that currently exist.
go_goroutines 12
# HELP go_memstats_alloc_bytes ...
go_memstats_alloc_bytes 2.866544e+06
# HELP process_cpu_seconds_total ...
process_cpu_seconds_total 0.06
```

### 4. 端到端冒烟

```bash
TOKEN=$(curl -sS -X POST http://127.0.0.1:8088/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"demo","password":"demo123"}' \
  | python -c "import sys,json;print(json.load(sys.stdin)['token'])")

curl -X POST http://127.0.0.1:8088/v1/world/move \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"player_id":"ebf2824b-1671-401c-8c0a-30675090336b",
       "from_tile_id":"tile_0_0","to_tile_id":"tile_1_0",
       "x":150.0,"y":50.0}'
# → {"player_id":"...","current_tile_id":"tile_1_0","x":150.0,"y":50.0,"ts_ms":...}

docker exec aicity-pg psql -U aicity -d aicity \
  -c "SELECT player_id, tile_id, x, y FROM player_position;"
# → ebf2824b-... | tile_1_0 | 150 | 50   ← 由订阅者 UpsertPosition 落库
```

### 5. tile 从 PG 加载

```bash
curl http://127.0.0.1:50052/v1/tiles/tile_0_0
# → { id:"tile_0_0", center_x:50, center_y:50, size:100,
#     buildings:[{id:"bldg_tavern_0_0", kind:"Tavern", polygon:[...]},
#                {id:"bldg_plaza_0_0",  kind:"Plaza",  polygon:[...]}],
#     npc_ids:["npc_wang_boss_001"], player_ids:[],
#     lod_level:"CBD" }
```

## 三、本次踩到的坑（避免重复踩）

| # | 问题 | 解决 |
|---|---|---|
| 1 | `sqlx 0.8` 链上 `base64ct 1.8.3` 要 `edition2024`，Rust 1.82 不支持 | 降到 `sqlx 0.7` |
| 2 | `sqlx 0.7` 仍拉 `icu_properties_data 2.3.0`（edition2024） | 砍 `chrono` feature |
| 3 | `sqlx 0.7` 仍拉 `parking_lot_core 0.9`，Windows + GNU toolchain 缺 `dlltool.exe` | 改 `runtime-tokio-native-tls` |
| 4 | 还是过不去；`sqlx` 链越来越深 | **彻底放弃 sqlx**，手写 `pg_client.rs`（与 Sprint 1.5 弃 `redis-rs` 同思路） |
| 5 | 升 `stable` Rust 后 `cc-rs` 找不到 `gcc.exe` | 回退到 `rust-toolchain.toml` 的 1.82，手写 PG 客户端无需 native dep |
| 6 | PG 容器走 `scram-sha-256`（默认），手写客户端只支持 trust | 改 `pg_hba.conf` 加 `host all all 172.17.0.0/16 trust`（注意顺序：放在 `host all all all scram-sha-256` **之前**，否则被 catch-all shadow） |
| 7 | `pg_hba_file_rules` 中 `172.17.0.0 trust` 放 line 103，line 100 `host all all all scram-sha-256` 先匹配 → 没起作用 | 把 scram 行注释掉，改成 subnet-specific rules |
| 8 | `pgxpool` 看到的是 docker bridge 的源 IP（172.17.0.1），不是 `127.0.0.1` | `pg_hba.conf` 加 172.17.0.0/16 子网 trust |
| 9 | `prometheus.MustRegister(NewGoCollector())` 报 `duplicate metrics collector registration`（构造时已自动注册到 default registry） | 改成 `_ = collectors.NewGoCollector()`，依赖构造副作用 |
| 10 | `MutexGuard<BufReader<...>>` 不直接满足 `AsyncReadExt` bound | 在调用处 `&mut *rguard` 显式 deref |

## 四、未做（明确留给 Sprint 3+）

### Sprint 3 候选

- [ ] **gRPC server (tonic)**：proto (`packages/proto/*.proto`) 已就位，main.rs 仍是 TODO；Sprint 3 接入，让 agent-core / ai-core 直接走 gRPC（REST 仅给 web 端）
- [ ] **Redis 订阅端反向通知**：当前 world-engine 单向 publish；Sprint 3 接入 `aicity:world:rebalance` 频道，多 world-engine 实例间同步玩家分布
- [ ] **PG 客户端 MD5 / SCRAM-SHA-256 鉴权**：当前只信任 docker bridge 子网；生产要支持任意来源
- [ ] **PG 客户端 binary format + prepared statement**：当前 Simple Query 文本格式，列多时浪费；接 EPgType / BinaryReceive 后性能更好
- [ ] **world-engine rust-toolchain 升 stable**：sqlx 仍要等 toolchain 升级才能用；当前手写客户端只够 Simple Query

### 待优化（非阻塞）

- [ ] `/metrics` 加 `service_info` label（service_name、version）便于 Prometheus 拉多实例
- [ ] `tile_loader` 加 SQL `ORDER BY` 之外的可配置过滤（如 `LIMIT 1000` 防全表）
- [ ] `RedisStats` 持久化到 OTLP，便于跨进程查询
- [ ] `world-engine /readyz` 加细粒度：`pg_last_query_ms`（最近一次 tile reload 耗时）
- [ ] `pg_hba.conf` 走 Docker 启动脚本自动注入，而不是 `docker exec` 手工改

## 五、变更清单

```
ai-city/packages/proto/pg-schema.sql                       (M)  +tile 表 + seed + schema_version 2.3.0
ai-city/apps/world-engine/Cargo.toml                       (M)  -sqlx 注释；+pg_client/tile_loader/metrics 注释
ai-city/apps/world-engine/Cargo.lock                       (M)  indexmap 2.14→2.6 兼容 Rust 1.82
ai-city/apps/world-engine/src/lib.rs                       (M)  +pub mod pg_client/tile_loader/metrics
ai-city/apps/world-engine/src/main.rs                      (M)  +DATABASE_URL → load_from_pg；fallback default_world
ai-city/apps/world-engine/src/world_grid.rs                (M)  +WorldGrid::with_tiles
ai-city/apps/world-engine/src/rest.rs                      (M)  /healthz liveness; +/readyz; +/metrics Prometheus
ai-city/apps/world-engine/src/pg_client.rs                 (A)  new — hand-rolled Simple Query client
ai-city/apps/world-engine/src/tile_loader.rs               (A)  new — SQL → HashMap<String, Tile>
ai-city/apps/world-engine/src/metrics.rs                   (A)  new — Prometheus text format renderer
ai-city/apps/api-gateway/internal/router/router.go         (M)  +/metrics route
ai-city/apps/api-gateway/cmd/main.go                       (M)  +NewGoCollector + NewProcessCollector
```

## 六、下一步建议

按依赖顺序：

1. **把 Sprint 2 推送到 origin/main**
2. **Sprint 3 规划**：(a) gRPC 接入 — proto 已在；(b) Redis 反向同步 — 单机不用，多机部署前必须
3. **CI 接入**：GitHub Actions → `cargo test` + `go test` + Prometheus scrape 健康度
4. **真实场景 E2E**：Playwright 模拟 web 玩家登录 + 移动 + 检查 PG 行 + 检查 /metrics

---

> **方法论沉淀**：项目第二次因为 native 依赖链（redis-rs → sqlx → parking_lot/dlltool）在 Windows + Rust 1.82 上崩溃而手写客户端。短期止血有效，长期需要：(a) 升 rust-toolchain 到 stable；(b) CI 加 `cargo check --target x86_64-pc-windows-gnu` 与 `x86_64-pc-windows-msvc` 双 target；(c) 任何新依赖先在两个 toolchain 上都跑过 `cargo check`。