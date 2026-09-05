# Sprint 5 复盘

> 范围：**a2a-gateway MVP —— A2A 联邦协议 gRPC server（RegisterCard / Discover / SendMessage / Stream）**
>
> 完成时间：2026-09-05
>
> 提交：
> - `feat(sprint5): a2a-gateway MVP — RegisterCard/Discover/SendMessage/Stream`

## 一、本次交付

### a2a-gateway（Go）

| 项 | 说明 |
|---|---|
| Proto 桩 | `packages/proto/gen/go/a2a/v1/{a2a.pb.go,a2a_grpc.pb.go}` —— `protoc` 生成，import path 与 go_package 对齐 |
| 注册表 | `internal/a2asrv/registry.go`：`sync.RWMutex + map[string]*AgentCard`，幂等覆盖，cityFilter MVP 忽略 + warn |
| Service | `internal/a2asrv/service.go`：4 RPC 全实现，F_001/F_003 走 gRPC InvalidArgument，F_004/F_005 走 `MessageResponse.Error`，Stream echo swap from/to + type="event" + 清空 signature |
| cmd | `cmd/main.go`：监听 `A2A_GRPC_ADDR`（默认 `127.0.0.1:50061`），SIGINT → GracefulStop（5s 超时 fallback Stop） |
| Smoke | `cmd/a2a_smoke/main.go`：独立二进制，5 检查项（Register × 2 / Discover / SendMessage ok+F_004 / Stream × 3） |
| 单测 | **7 通过 / 0 failed**（Registry：6 + 并发 -race-friendly 1） |
| 集成测 | **9 通过 / 0 failed**（bufconn 假 server；2 RPC × 4 + Stream 1） |
| E2E | `cmd/a2a_smoke` 全 5 项 OK（详见 §二） |

### 错误码体系（MVP）

| 触发 | 返回位置 | 形式 |
|---|---|---|
| `RegisterCard` 缺字段 | gRPC status | `codes.InvalidArgument` + msg `"F_001:agent_id and name required"` |
| `RegisterCard` 重复 | accepted:true | 不暴露 F_002（MVP：幂等覆盖即成功） |
| `Discover` 空 capability | gRPC status | `codes.InvalidArgument` + msg `"F_003:capability required"` |
| `SendMessage` 收件方未注册 | `MessageResponse.Error` | `"F_004:recipient not found"`（delivered:false） |
| `SendMessage` 发件方未注册 | `MessageResponse.Error` | `"F_005:sender not registered"`（delivered:false） |
| `SendMessage` 成功 | `MessageResponse.Error` | `""`（delivered:true） |

## 二、E2E 输出

```
$ go test ./...
ok  	github.com/aicity/a2a-gateway/internal/a2asrv	0.151s
（17 subtests 全过，含 bufconn 9 + Registry 7）

$ A2A_GRPC_ADDR=127.0.0.1:50061 go run ./cmd/main.go &
2026/09/05 10:15:56 a2a-gateway starting gRPC on 127.0.0.1:50061
2026/09/05 10:15:56 a2a-gateway ready (registry size=0)

$ go run ./cmd/a2a_smoke
[a2a_smoke] dial 127.0.0.1:50061 ...
[OK]   RegisterCard alice / bob
[OK]   Discover capability=chat → 2 cards
[OK]   SendMessage alice → bob delivered=true
[OK]   SendMessage alice → ghost delivered=false error=F_004
[OK]   Stream echo 3 messages

[OK] all 5 a2a_smoke checks passed against 127.0.0.1:50061
```

## 三、关键决策

| 项 | 决策 | 理由 |
|---|---|---|
| 协议 | gRPC only（不动 gin HTTP） | A2A §20.1 明确 gRPC；MVP HTTP gateway 是 Sprint 6 适配层 |
| 端口 | `:50061` | 与 world-engine 50051 / api-gateway 8088 类比；环境变量 `A2A_GRPC_ADDR` |
| 注册表 | `sync.RWMutex + map`（纯内存） | MVP 不持久化；Sprint 6+ 接 PG |
| Inbox | 不消费（SendMessage 仅校验双方 + 返成功） | MVP 不路由；Stream 仅 echo；Sprint 5.5+ adapter 替换 |
| F_001/F_003 | gRPC `codes.InvalidArgument` + msg `"F_XXX:..."` | proto `RegisterResponse` / `DiscoverResponse` 无 Error 字段 |
| F_004/F_005 | `MessageResponse.Error` 字符串 | proto 已有 Error 字段，调用方可读 error 即可（不需查 gRPC status） |
| F_002 | 不暴露（accepted:true 即成功） | 幂等覆盖即语义成功；调用方无需区分 |
| Stream echo | swap from/to + type="event" + 清空 signature | 占位；后续由 adapter 替换为真路由 |
| proto 输出路径 | `gen/go/a2a/v1/`（与 go_package 对齐） | 用 `protoc --go_out=.../a2a/v1 ... --go_opt=paths=source_relative` 让目录与路径一致 |
| 不在 go.mod 加 `github.com/aicity/proto` | 依赖 `go.work` 解析 | 加 require 会触发 `v0.0.0` 远程校验 → 当前环境 github.com 网络超时；workspace 已 include packages/proto，无需重复 require |

## 四、踩坑

1. **`protoc --go_opt=paths=source_relative` 目录对齐**
   - 坑：`a2a.proto` 在 `packages/proto/a2a.proto`（根），world.proto 同位置
   - 现象：默认输出 `gen/go/a2a.pb.go`，与 `world.pb.go` 同目录 → Go 报 `found packages a2av1 and worldv1 in ...`
   - 解：`--go_out=packages/proto/gen/go/a2a/v1` 让 protoc 把相对路径 `a2a.proto` 当作 `a2a/v1/` 看待（实际只放 a2a.pb.go 在子目录）
   - 副效果：`--go_opt=paths=source_relative` 会忽略 `M` 映射；通过调整 `--go_out` 的目标子目录最干净

2. **`go.work` + `github.com/aicity/proto` require 冲突**
   - 坑：在 a2a-gateway/go.mod 加 `require github.com/aicity/proto v0.0.0` → Go 远程拉取 v0.0.0 失败（github.com 网络超时 21s）
   - 解：参考 api-gateway 写法 —— go.mod 不 require proto；workspace `use ./packages/proto` 已包含

3. **`GOPROXY=off` 在 workspace 模式仍报 `module lookup disabled`**
   - 现象：注释 require 后想用 GOPROXY=off 跳过网络，但 Go 仍查 proxy
   - 解：goproxy.cn 已配 → 网络可达时 `go build ./...` 直接过；离线场景需把 grpc 拉入 vendor（本 Sprint 不必）

## 五、变更清单

### 新增
- `packages/proto/gen/go/a2a/v1/a2a.pb.go`（生成，需 `git add -f`）
- `packages/proto/gen/go/a2a/v1/a2a_grpc.pb.go`（生成，需 `git add -f`）
- `apps/a2a-gateway/internal/a2asrv/registry.go`
- `apps/a2a-gateway/internal/a2asrv/registry_test.go`
- `apps/a2a-gateway/internal/a2asrv/service.go`
- `apps/a2a-gateway/internal/a2asrv/service_test.go`
- `apps/a2a-gateway/cmd/a2a_smoke/main.go`

### 修改
- `apps/a2a-gateway/cmd/main.go`：占位 → 真正 gRPC server
- `apps/a2a-gateway/go.mod`：移除 jwt/redis（暂未用），加 grpc v1.66.0
- `apps/a2a-gateway/README.md`：端口 8083 + 新增 gRPC 50061 + 启动/测试命令
- `go.work.sum`：补 grpc + 间接依赖 checksum

## 六、不在本次范围

按依赖顺序留待后续 Sprint：
1. **Sprint 5.5**：ed25519 signature 校验（`AgentCard.auth["ed25519:..."]` 验签 Message.signature）+ openClaw/workbuddy adapter 框架 — 见 [Sprint 5.5 复盘](./SPRINT-5.5.md)
2. **Sprint 6**：a2a-restart HTTP gateway（gin adapter → gRPC client），让外部联邦通过 HTTP 接入
3. **Sprint 6+**：PG 持久化 agent_card + inbox（重启保留 + 跨实例联邦发现）

## 七、配套规范

- **Canonical 签名形式**：[docs/06-A2A-canonical.md](./06-A2A-canonical.md) — 8 字段固定 JSON、payload 用 base64.RawStdEncoding、signature 字段在 canonical 里缺席
