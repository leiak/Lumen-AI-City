# a2a-gateway

> **职责**：A2A v0.1 联邦协议 + Agent Card + Envelope + openClaw/workbuddy 适配器
>
> **关键文档**：[docs/06-A2A协议.md](../../docs/06-A2A协议.md) 全文
>
> **复盘**：[docs/SPRINT-5.md](../../docs/SPRINT-5.md)

## 端口

| 类型 | 端口 | 环境变量 | 说明 |
|---|---|---|---|
| gRPC | `50061` | `A2A_GRPC_ADDR`（默认 `127.0.0.1:50061`） | Sprint 5 MVP；A2AGateway 4 RPC |
| HTTP | `8083` | `A2A_GATEWAY_PORT` | 占位，HTTP gateway 留给 Sprint 6（a2a-restart 适配层） |

## 协议能力（Sprint 5 MVP）

- Agent Card 注册（`RegisterCard`） + 联邦发现（`Discover`）
- Envelope 投递（`SendMessage`） + 双向流（`Stream`，MVP echo 占位）
- A2A 错误码 F_001/F_002/F_003/F_004/F_005（§20.10）
- 内存注册表（重启即清）

## 协议能力（Sprint 5.5+ 后续）

- mTLS + ed25519 签名校验（§20.11-13）
- openClaw / workbuddy 适配器（§20.14）
- HTTP gateway（a2a-restart）
- PG 持久化（agent_card + inbox）

## 启动

```bash
# gRPC server（默认 127.0.0.1:50061）
A2A_GRPC_ADDR=127.0.0.1:50061 go run ./cmd/main.go

# E2E smoke（连运行中的 server）
go run ./cmd/a2a_smoke
```

## 测试

```bash
go test ./...                  # 全部单元 + bufconn 集成测试
go test -v -run TestService_   # 仅 Service 测试
go test -v -run TestRegistry_  # 仅 Registry 测试
```
