# ws-gateway

> **职责**：WebSocket 长连接 / 心跳 / 消息分发
>
> **关键文档**：[docs/04-API设计.md §18.3](../../docs/04-API设计.md)

## 端口

`8082`

## 协议

升级路径：`/ws`

## 消息格式（JSON）

```json
{
  "type": "npc_dialogue",
  "trace_id": "uuid",
  "ts_ms": 1700000000000,
  "payload": { ... }
}
```
