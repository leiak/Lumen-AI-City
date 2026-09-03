# packages/proto

> **职责**：跨服务 gRPC 协议定义（**所有微服务强类型通信的契约之源**）
>
> **关键文档**：[docs/04-API设计.md §18.3](../../docs/04-API设计.md) / [docs/11-技术细节与玩法模式.md §A.3](../../docs/11-技术细节与玩法模式.md)

## 文件清单

| 文件 | 服务 | 说明 |
|---|---|---|
| `world.proto` | World Engine | Tile / 移动 / 路径 |
| `agent.proto` | Agent OS | 决策 / 状态 / 对话 |
| `memory.proto` | Memory Service | 三层记忆 CRUD |
| `saga.proto` | Saga Orchestrator | 分布式事务 |
| `a2a.proto` | A2A Gateway | 联邦协议（§20） |
| `notification.proto` | Notification Engine | Push 通知 |

## 工具

```bash
# 安装 buf
brew install buf  # macOS
# 或: go install github.com/bufbuild/buf/cmd/buf@latest

# 生成代码
buf generate

# Lint 检查
buf lint

# 兼容性检查
buf breaking --against '.git#branch=main'
```

## 版本策略

- SemVer
- buf push 到 BSR（Buf Schema Registry）
- 配套 GitHub Release
