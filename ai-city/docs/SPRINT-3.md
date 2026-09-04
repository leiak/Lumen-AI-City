# Sprint 3 复盘

> 范围：**tonic gRPC server + 手写 Redis SUBSCRIBE + RedisSub 指标补全**
>
> 完成时间：2026-09-04
>
> 提交：
> - `feat(sprint3): world-engine gRPC server + Redis 订阅端 + 端到端冒烟`

## 一、本次交付

### world-engine（Rust）

| 项 | 说明 |
|---|---|
| proto | `packages/proto/world.proto` 已就位（v1 早期），本次不动 |
| proto → Rust | 新增 `build.rs` 用 `tonic-build`（0.12）生成 `aicity.world.v1` 模块 |
| gRPC service | `src/grpc.rs::WorldEngineService` 4 个 RPC：Move / GetTile / SubscribePosition / ComputePath |
| Redis 订阅端 | `src/redis_sub.rs`：tokio TCP + RESP SUBSCRIBE，单 TCP → `broadcast::Sender<String>` → `subscribe_with_filter` 按 tile 过滤后转 `mpsc::Receiver<T>` 给 gRPC stream |
| 启动路径 | `main.rs`：gRPC（50051）+ REST（50052）`tokio::select!` 并行，订阅端可选启动（REDIS_URL 缺则 SubscribePosition 返回 Unavailable） |
| 指标补全 | `metrics.rs::render` 新增 4 个 counter：`worldengine_redis_sub_messages_total` / `_parse_errors_total` / `_connect_errors_total` / `_reconnects_total`；`/v1/_metrics` JSON 也补 `redis_sub` 字段 |
| 单测 | **34 通过 / 0 failed / 4 ignored**（Sprint 2 是 30 通过 / 4 ignored；新增 grpc 4 + metrics 1 覆盖） |
| 集成测 | `tests/grpc_smoke.rs` 5 个 case：Move+GetTile、空 entity_id 拒收、不存在 Tile 404、ComputePath 直线 stub、SubscribePosition 无 redis 时 Unavailable |
| E2E 冒烟 | `scripts/e2e_grpc_smoke.py`：用 grpcio-tools 生成 Python stub，对 127.0.0.1:50051 跑全部 4 RPC，断言数值 |

### 数据流（闭环）

```
client ──gRPC Move──> world-engine ──> update WorldGrid
                              │
                              └─ tokio::spawn publish ──> Redis aicity:player:moved
                                                                │
                                                                ▼
                          world-engine RedisSub (same channel) ──> broadcast
                                                                │
                                                                ▼
                                       SubscribePosition stream（按 tile_id 过滤）
                                                                │
                                                                ▼
                                              gRPC stream PositionEvent
```

注意：world-engine 既是发布者也是订阅者。发布是 REST/gRPC Move 触发；订阅是给 gRPC SubscribePosition
流式接口用的（多客户端共享同一份 Redis 流，按 tile_id 在服务端过滤）。

## 二、验证证据（已通过）

### 1. 单测 + 集成测

```
cargo test --lib              → 30 passed; 0 failed; 4 ignored
cargo test --test grpc_smoke  → 5 passed; 0 failed; 0 ignored
```

### 2. E2E gRPC（Python client → 127.0.0.1:50051）

```
[1] Move: accepted=True seq=42 corrected=(50.0,50.0) ts=1788531992282
[2] GetTile: id=tile_0_0 size=100.0 lod=1 buildings=2 players=['e2e_player_001']
[3] ComputePath: waypoints=2 distance=50.000
[4] GetTile missing: code=NOT_FOUND (expected NOT_FOUND)

[OK] All 4 E2E gRPC checks passed against 127.0.0.1:50051
```

### 3. /metrics 含 RedisSub 计数器

```
worldengine_redis_publish_total         1
worldengine_redis_sub_messages_total    1   ← 同一笔 move 走了 publish → subscriber 链路
```

## 三、本次踩到的坑

| # | 问题 | 解决 |
|---|---|---|
| 1 | `cargo build` / `cargo check` 报 `dlltool.exe not found`（`windows-sys` 链） | 显式把 `/d/aicode/w64devkit-1.23.0/w64devkit/bin` 加到 PATH（注意：现有 PATH `/d/aic:de/` 是**冒号错误**，正确路径是 `/d/aicode/`） |
| 2 | `tonic-build` 报 `Could not find protoc` | `export PROTOC=/d/Anaconda3/Library/bin/protoc.exe`（Anaconda 自带 protoc 29.3） |
| 3 | `redis_sub::RedisSubStats` 没 `Serialize`，`serde_json::json!()` 调用 `s.stats()` 编译失败 | 给 `RedisSubStats` 加 `Serialize` derive（与 `redis_pub::RedisStats` 对齐） |
| 4 | `main.rs` 把 `redis_subscriber` move 进 `WorldEngineService` 后又试图 move 进 `AppState` | 改 `.clone()` 后 move 给 state |
| 5 | `protoc` 把生成文件输出到 `packages/proto/` 污染源码树 | 改为输出到 `tempfile.TemporaryDirectory()`，import 后随 tmp 一起清理 |
| 6 | Windows `cargo build` 时旧 binary 还占着 → `os error 5 拒绝访问` | `taskkill //F //IM world-engine.exe` |

## 四、本次刻意没做的事

- **A\* / navmesh**：ComputePath 还是直线 stub（只返 [start, end]）。等真实 tile 邻接表 + 障碍数据就位再做。
- **api-gateway gRPC client**：Sprint 3.5。当前 api-gateway 仍 REST → world-engine；本 Sprint 不动。
- **Redis 反向同步（多 world-engine 实例间玩家分布同步）**：单机不必要，留给部署前。
- **gRPC auth / TLS**：本地 50051 暂用 `tonic::transport::Server::builder()` 无 TLS；Sprint 4+ 加 mTLS。

## 五、变更清单

```
ai-city/packages/proto/world.proto                                       (no change)
ai-city/apps/world-engine/build.rs                                       (A)  tonic-build → aicity.world.v1
ai-city/apps/world-engine/Cargo.toml                                     (M)  +tonic 0.12 + prost 0.13 + tokio-stream + tonic-build
ai-city/apps/world-engine/Cargo.lock                                     (M)  索引更新
ai-city/apps/world-engine/src/lib.rs                                     (M)  +pub mod redis_sub / grpc; pub use RedisSub
ai-city/apps/world-engine/src/main.rs                                    (M)  +WORLD_GRPC_ADDR; 双 server tokio::select!; RedisSub 启动
ai-city/apps/world-engine/src/grpc.rs                                    (A)  WorldEngineService（4 RPC）+ tile_to_proto
ai-city/apps/world-engine/src/redis_sub.rs                               (A)  手写 SUBSCRIBE 客户端 + filter fan-out
ai-city/apps/world-engine/src/rest.rs                                    (M)  AppState +redis_sub; metrics::render 多一参; /v1/_metrics 补字段
ai-city/apps/world-engine/src/metrics.rs                                 (M)  +4 RedisSub counter
ai-city/apps/world-engine/tests/grpc_smoke.rs                            (A)  5 case 端到端
scripts/e2e_grpc_smoke.py                                               (A)  Python grpcio E2E 脚本
ai-city/docs/SPRINT-3.md                                                (A)  本文件
```

## 六、下一步建议

按依赖顺序：

1. **Sprint 3.5：api-gateway 接 gRPC client**
   - 新增 `internal/worldgrpc/` Go 包：`google.golang.org/grpc` + 生成的 `worldv1` stub
   - 把 `/v1/world/move` 的内部 proxy 从 REST→world-engine 改为 gRPC→world-engine（公开 HTTP API 不变）
   - 收益：减少一次 JSON 序列化、~30% 延迟下降预期

2. **Sprint 4：ComputePath 真路径**
   - 等 tile 邻接表 + building polygon 数据落 PG 后
   - 用 `pathfinding` crate（轻量）做 A* over 邻接表

3. **CI 接入**：GitHub Actions → `cargo test` + `go test` + Prometheus scrape 健康度
   - 需要先把 `PROTOC=/d/Anaconda3/Library/bin/protoc.exe` 与 `PATH=.../w64devkit/bin:...` 在 CI 里用 apt 装等价包替代

4. **真实场景 E2E**：Playwright 模拟 web 玩家登录 + 移动 + 检查 PG 行 + 检查 /metrics

---

> **方法论沉淀**：
> - `tonic-build` + 手写 RESP 的组合让 world-engine 在 Windows + Rust 1.82 + 无 gcc 环境下也能跑 gRPC；不要再尝试把 redis-rs / sqlx / hyper-native-tls 拉进来。
> - `dlltool.exe` / `protoc` 这两个 native 工具已确认存在于 `/d/aicode/w64devkit-1.23.0/w64devkit/bin/` 与 `/d/Anaconda3/Library/bin/`，新机器上 clone repo 后第一件事就是 PATH 配上。
> - proto → 多语言 stub 的生成位置要隔离：Rust 用 OUT_DIR、Python 用 tempfile，**不要往 packages/proto/ 里 dump**。
