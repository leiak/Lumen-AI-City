# packages/event-schemas

> **职责**：Kafka 事件的 Avro Schema 定义
>
> **注册中心**：Confluent Schema Registry

## 主题清单

| Schema | Topic | 发送方 | 订阅方 |
|---|---|---|---|
| `perception_event.avsc` | `agent.perception` | Agent OS | Memory Service, Analytics |
| `decision_event.avsc` | `agent.decision` | Agent OS | Analytics, Observability |
| `saga_event.avsc` | `saga.events` | Saga Orchestrator | Dashboard, Analytics |
| `federation_event.avsc` | `federation.events` | A2A Gateway | Analytics |
| `notification_event.avsc` | `notification.events` | Notification Engine | Analytics |

## 兼容性

- 启用 BACKWARD 兼容（只能加字段，不能删）
- CI 自动跑 `maven-avro-plugin` 兼容性检查
