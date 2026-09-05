# a2a-gateway

> **职责**：A2A v0.1 联邦协议 + Agent Card + Envelope + openClaw/workbuddy 真 HTTP 适配器
>
> **关键文档**：[docs/06-A2A协议.md](../../docs/06-A2A协议.md) 全文
>
> **复盘**：[docs/SPRINT-5.md](../../docs/SPRINT-5.md) · [docs/SPRINT-6.md](../../docs/SPRINT-6.md)

## 端口

| 类型 | 端口 | 环境变量 | 说明 |
|---|---|---|---|
| gRPC | `50061` | `A2A_GRPC_ADDR`（默认 `127.0.0.1:50061`） | Sprint 5+；A2AGateway 4 RPC |
| HTTP | `8083` | `A2A_HTTP_ADDR`（默认 `127.0.0.1:8083`） | **Sprint 6**；in-process 双协议网关 |

## 协议能力（Sprint 5 MVP）

- Agent Card 注册（`RegisterCard`） + 联邦发现（`Discover`）
- Envelope 投递（`SendMessage`） + 双向流（`Stream`，EchoAdapter）
- A2A 错误码 F_001/F_002/F_003/F_004/F_005（§20.10）
- 内存注册表（重启即清）

## 协议能力（Sprint 5.5）

- ed25519 签名校验（§20.11-13）：`auth["ed25519"]` 公钥注册 + 重放窗口（默认 5min）
- Adapter 框架（§20.14）：`Dispatcher` 选路 + `EchoAdapter` 兜底
- 错误码增量：F_006（pubkey）/ F_007（signature）/ F_008（ts_ms）/ F_009（provider 路由）

## 协议能力（Sprint 6）

- **HTTP gateway**（同进程双协议）：4 端点 + Bearer 鉴权（可选）+ trace_id 透传
- **真 HTTP outbound Adapter**：`openclaw` / `workbuddy` → `POST {recipient.URL}/inbox`
  - 共享 `*http.Client{Timeout: 5s}` + `Transport.MaxIdleConnsPerHost=4`
  - 200 + JSON → 解 reply；204 → fire-and-forget；4xx/5xx/超时 → F_010
- **错误码增量**：F_010（upstream 不可达 / 4xx 5xx / reply 解码失败）
- **F_001-F_010 → HTTP status 映射**：

  | F-code | HTTP | 含义 |
  |---|---|---|
  | F_001 | 400 | agent_id/name 缺失 |
  | F_003 | 400 | capability 为空 |
  | F_004 | 404 | 收件方未注册 |
  | F_005 | 401 | 发件方未注册 |
  | F_006 | 400 | pubkey 解析失败 |
  | F_007 | 401 | signature 失败 |
  | F_008 | 401 | ts_ms 出窗 |
  | F_009 | 400 | provider 路由失败 |
  | F_010 | 502 | upstream 不可达 |

## HTTP 路由

| Method | Path | Handler | 鉴权 |
|---|---|---|---|
| GET  | `/v1/healthz` | Healthz | 公开 |
| POST | `/v1/cards` | RegisterCard | Bearer* |
| GET  | `/v1/discover` | Discover | Bearer* |
| POST | `/v1/messages` | SendMessage | Bearer* |

*Bearer 仅当 `A2A_HTTP_API_KEY` 非空时启用；`/v1/healthz` 永远公开。

## 启动

```bash
# 双协议 server（默认 gRPC :50061 + HTTP :8083）
A2A_GRPC_ADDR=127.0.0.1:50061 \
A2A_HTTP_ADDR=127.0.0.1:8083 \
go run ./cmd/main.go

# 仅启动时启用 Bearer 鉴权（dev 留空）
A2A_HTTP_API_KEY=dev-secret ./cmd/main.go
```

## 端到端验证

```bash
# 1) gRPC smoke（12 项 Sprint 5+5.5）
A2A_GRPC_ADDR=127.0.0.1:50061 ./bin/a2a_smoke.exe

# 2) HTTP smoke（8 项 Sprint 6）
A2A_HTTP_ADDR=http://127.0.0.1:8083 ./bin/http_smoke.exe

# 3) curl 手动验证
curl -s http://127.0.0.1:8083/v1/healthz
curl -s -X POST http://127.0.0.1:8083/v1/cards \
  -H 'Content-Type: application/json' \
  -d '{"agent_id":"alice","name":"Alice","provider":"aicity","capabilities":["chat"]}'
curl -s 'http://127.0.0.1:8083/v1/discover?capability=chat'
curl -s -X POST http://127.0.0.1:8083/v1/messages \
  -H 'Content-Type: application/json' \
  -d '{"message_id":"m1","from_agent_id":"alice","to_agent_id":"bob","type":"request","payload":"aGk=","ts_ms":0}'
```

## 测试

```bash
go test ./...                                # 全部单元 + bufconn + httptest 集成测试
go test -v -run TestService_                 # 仅 Service gRPC 测试
go test -v -run TestRouter_ ./internal/httpgw  # 仅 HTTP gateway 测试
go test -v -run TestHTTPAdapter_             # 仅 HTTPAdapter 测试
go test -v -run TestFCodeToHTTP_             # 仅 errmap 错误码映射
```

## 环境变量汇总

| 变量 | 默认 | 说明 |
|---|---|---|
| `A2A_GRPC_ADDR` | `127.0.0.1:50061` | gRPC 监听地址 |
| `A2A_HTTP_ADDR` | `127.0.0.1:8083` | HTTP 监听地址 |
| `A2A_HTTP_API_KEY` | 空（关鉴权） | HTTP Bearer token；非空时强制校验 |
| `A2A_REPLAY_WINDOW_SEC` | `300` | ed25519 重放窗口秒数 |

## 不在 Sprint 6 范围

- PG 持久化（agent_card / inbox）→ Sprint 7
- ACL + 跨城邦路由 → Sprint 7+
- AgentCard 自签 / CA → Sprint 8+
- HTTP 流式镜像（SSE / WebSocket）→ Sprint 8+（确认需求后）
