# saga-worker

> **职责**：从 Kafka 消费 Saga step 任务并执行（带重试 + DLQ）

## Kafka Topic

- `saga.steps` - 入队 step
- `saga.steps.dlq` - 死信队列

## Runtime Contract

- 消费组默认为 `saga-worker-group`，多副本共享分区负载，不按 `WORKER_ID` 拆分 group。
- `KAFKA_TOPIC` / `KAFKA_DLQ_TOPIC` / `KAFKA_GROUP` 可覆盖。
- 处理失败最多重试 3 次；最终失败写入 DLQ，消息体包含 Base64 原始 payload 和 error。
- 每条消息处理成功或 DLQ 发布成功后才手动 commit，避免 poison message 阻塞分区。
