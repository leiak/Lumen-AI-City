# Sprint 3.5 复盘

> 范围：**api-gateway 接 world-engine gRPC client（替代 /v1/world/move 的 REST proxy 路径）**
>
> 完成时间：2026-09-04
>
> 提交：
> - `feat(sprint3.5): api-gateway gRPC client — proto Go 生成 + worldgrpc 包 + /v1/world/move 走 gRPC`

## 一、本次交付

### api-gateway（Go）

| 项 | 说明 |
|---|---|
| proto Go 生成 | `packages/proto/gen/go/world.pb.go` + `world_grpc.pb.go`（package `worldv1`）；用 `protoc-gen-go` 1.36.12 + `protoc-gen-go-grpc` 1.6.2 |
| proto 模块化 | 新增 `packages/proto/go.mod`（module `github.com/aicity/proto`），加入根 `go.work` 的 `use()` |
| gRPC client | `internal/worldgrpc/client.go`：封装 `grpc.ClientConn` + `worldv1.WorldEngineClient`；提供 `NewClient(addr)` / `Close()` / `Move` / `GetTile` / `ComputePath` / `IsGRPCUnavailable` |
| 新 handler | `internal/handlers/world_move.go`：`WorldMoveHandler.Move` 把 HTTP body 映射为 `MoveRequest`、调 gRPC、把 `MoveResponse` 重新序列化为 REST 形态（公共 API 不变） |
| gRPC code → HTTP | `InvalidArgument→400` / `NotFound→404` / `Unavailable→503` / `DeadlineExceeded→504` / 其它 `→502` |
| 路由切换 | `POST /v1/world/move` → `WorldMoveHandler.Move`（原 `worldProxy.Proxy`）；`/v1/tiles*` 仍走 REST proxy（web 端 + 便于缓存） |
| 启动路径 | `cmd/main.go` 在 redis 之后 dial gRPC（5s 超时），失败 `Fatal`；`defer worldClient.Close()` |
| 配置 | `internal/config` 新增 `WorldGRPCAddr`（env `WORLD_ENGINE_GRPC_ADDR`，默认 `127.0.0.1:50051`） |
| 单测 | **5 通过**（handlers 3：bad JSON 400 + code 映射表；worldgrpc 4：Move 成功 + 错误传播 + Unavailable 判断 + nil req） |
| E2E smoke | `cmd/grpc_smoke/main.go`：独立 Go 二进制，对 50051 跑 Move / GetTile / ComputePath；CI / 手动验收用 |

### 数据流（混合 REST + gRPC）

```
client ──POST /v1/world/move──> api-gateway ──gRPC Move──> world-engine :50051
   │                                                              │
   │                                                              ├─ update WorldGrid
   │                                                              └─ tokio::spawn publish ──> Redis
   │                                                                                          │
   │                                                                                          ▼
   │                                          api-gateway subscriber (go-redis) ──> PG player_position
   │
   └──GET /v1/tiles/...──> api-gateway ──HTTP──> world-engine :50052
```

## 二、验证证据

### 1. 单测

```
go test ./internal/handlers/... ./internal/worldgrpc/...
ok  	github.com/aicity/api-gateway/internal/handlers	2.477s
ok  	github.com/aicity/api-gateway/internal/worldgrpc	2.332s
```

### 2. E2E gRPC smoke（Go client → world-engine:50051）

```
[grpc_smoke] dial 127.0.0.1:50051 ...
[OK]   Move accepted, corrected=(150.0,50.0) ts=1788534802324
[OK]   GetTile tile_1_0 size=100 players=[grpc_smoke_player_001]
[OK]   ComputePath waypoints=2 distance=50.000

[OK] all 3 grpc_smoke checks passed against 127.0.0.1:50051
```

### 3. 编译

```
go build -o bin/api-gateway.exe ./cmd   →  35 MB exe
go build -o bin/grpc_smoke.exe ./cmd/grpc_smoke   →  通过
```

## 三、本次踩到的坑

| # | 问题 | 解决 |
|---|---|---|
| 1 | `protoc-gen-go` / `protoc-gen-go-grpc` 不在 PATH | `go install google.golang.org/protobuf/cmd/protoc-gen-go@latest` + `go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest` → 落到 `/c/Users/wma19/go/bin` |
| 2 | protoc 生成的文件路径与 `go_package` 不一致（输出 `gen/go/world.pb.go` 但 go_package 是 `gen/go/world/v1`） | 暂时接受 mismatch；import 路径用 `github.com/aicity/proto/gen/go`，包名用 `worldv1`。后续如要规范可建 `world/v1/world.proto` 子目录或加 `Mworld.proto=...` 映射 |
| 3 | `go mod tidy` 卡 10 分钟（opentelemetry/otel 大依赖链） | 跳过 tidy，用 `go build ./...` 直接构建 —— workspace 已把 `github.com/aicity/proto` 显式纳入 `use()`，无需 replace |
| 4 | `git` 不在默认 PATH，go.work 解析模块失败 | 把 `/mingw64/bin` 加 PATH（git.exe 在那里） |
| 5 | TestWorldMoveHandler 测试用例 `c` 传了 struct 不是 codes.Code | 改为 `c.grpc` |
| 6 | Docker daemon 没启动 → api-gateway 启动期 `pg ping failed` Fatal，无法做完整 HTTP→gRPC E2E | 写独立 `grpc_smoke` 二进制绕过 api-gateway，单独验证 gRPC 客户端到 world-engine 链路；HTTP→gRPC 完整 E2E 留待 docker 起来后补 |

## 四、本次刻意没做的事

- **mTLS**：grpc dial 仍用 `insecure.NewCredentials()`。生产需要 mTLS + CA 校验
- **circuit breaker**：`IsGRPCUnavailable` 工具函数已就位，但 handler 还没接入 `sony/gobreaker`（依赖已在 go.mod 里）。Sprint 4 接入
- **Retry / backoff**：gRPC 默认无重试；高频写场景需 `grpc.UnaryClientInterceptor` 加 metadata 透传 + 重试
- **公共 API 形态**：刻意保留 REST（前端 / 第三方 SDK 不用改）

## 五、变更清单

```
ai-city/go.work                                                  (M)  +packages/proto
ai-city/go.work.sum                                              (M)  +proto 依赖 hash
ai-city/packages/proto/go.mod                                    (A)  new — module github.com/aicity/proto
ai-city/packages/proto/go.sum                                    (A)  new
ai-city/packages/proto/gen/go/world.pb.go                        (A)  generated
ai-city/packages/proto/gen/go/world_grpc.pb.go                   (A)  generated
ai-city/apps/api-gateway/internal/worldgrpc/client.go            (A)  gRPC client wrapper
ai-city/apps/api-gateway/internal/worldgrpc/client_test.go       (A)  4 unit tests (bufconn)
ai-city/apps/api-gateway/internal/handlers/world_move.go         (A)  gRPC-based move handler
ai-city/apps/api-gateway/internal/handlers/world_move_test.go    (A)  3 unit tests (bad JSON + code map)
ai-city/apps/api-gateway/internal/config/config.go               (M)  +WorldGRPCAddr
ai-city/apps/api-gateway/internal/router/router.go               (M)  +WorldMoveHandler; move 路由切换
ai-city/apps/api-gateway/cmd/main.go                             (M)  +worldgrpc.NewClient + Close
ai-city/apps/api-gateway/cmd/grpc_smoke/main.go                  (A)  E2E smoke binary
ai-city/docs/SPRINT-3.5.md                                       (A)  本文件
```

## 六、下一步建议

按依赖顺序：

1. **补 HTTP→gRPC 完整 E2E**：等 docker daemon 起来 + PG 起来后跑一遍 `curl -X POST :8088/v1/world/move` → 验证 `source_channel=grpc` + world-engine 那侧收到 `gRPC move` log + Redis publish_total 计数 +1
2. **Sprint 4：ComputePath 真路径**（world-engine 侧，需要 tile 邻接表 + 障碍数据）
3. **circuit breaker**：把 `sony/gobreaker` 接到 WorldMoveHandler，world-engine 不可用时 503 + 缓存响应
4. **mTLS**：自签 CA + grpc.WithTransportCredentials(credentials.NewServerTLSFromCert(...))
5. **CI 接入**：GitHub Actions 跑 `cargo test` + `go test` + `./grpc_smoke`（需要先把 protoc + w64devkit 等价物在 CI runner 装好）

---

> **方法论沉淀**：
> - Go 的 proto 生成代码目录 `gen/go/` 与 `go_package` 不一致时仍能 import（import path vs package name 是分开的两个轴），不要为了对齐瞎挪文件
> - `go.work` 的 `use()` 是 monorepo 跨模块依赖最干净的写法；不要再写 `replace github.com/foo => ../foo`
> - Sprint 3.5 把 REST → gRPC 的切换点设在 api-gateway 单跳，对外 API 零改动 —— 这是迁移现有服务到 gRPC 的最小爆炸半径
