# saga-worker

> **职责**：从 Kafka 消费 Saga step 任务并执行（带重试 + DLQ）

## Kafka Topic

- `saga.steps` - 入队 step
- `saga.steps.dlq` - 死信队列
