# ADR-0004: Saga 编排采用 Kafka + Orchestrator/Worker 模式

> **状态**：Accepted
>
> **日期**：2026-09-03
>
> **决策人**：@tech-lead @backend-lead
>
> **影响范围**：`apps/saga-orchestrator/`, `apps/saga-worker/`, `packages/event-schemas/`

## 背景

城市剧本（新年庆典、酒馆聚会等）需要分布式事务支持：
- 多个步骤可能跨服务
- 失败时必须补偿
- 不能阻塞玩家

## 决策

**采用 Saga 模式 + Kafka 作为 step 队列 + 独立 Orchestrator/Worker**。

## 备选方案

### 方案 A：2PC / XA 事务
- ✅ 强一致
- ❌ 阻塞玩家（同步等待），LLM 慢响应会拖死
- ❌ Saga 已经覆盖业务需求，2PC 过度

### 方案 B：Workflow 引擎（Temporal / Cadence）
- ✅ 成熟方案
- ❌ 引入外部依赖，学习成本高
- ❌ 与 LLM 异步调用整合不顺

### 方案 C：纯 DB 状态机
- ✅ 简单
- ❌ 单 DB 单点故障
- ❌ 跨服务协调困难

## 影响

### 正面
- Kafka 天然支持异步、可重试、DLQ
- Orchestrator / Worker 解耦，可独立伸缩
- 与已有 CDC、Kafka 生态一致

### 负面
- Saga 调试比同步调用难（需要 trace_id 关联）
- 最终一致性，玩家可能看到"中间态"

### 缓解
- 强制每个 step 必须有 trace_id
- Saga Dashboard 实时可见（§32.7）
- 玩家侧用乐观 UI 隐藏中间态

## 实施计划

- [x] Saga Orchestrator 骨架
- [x] Saga Worker（消费 + 重试 + DLQ）
- [x] Avro schema（saga_event.avsc）
- [x] Saga Dashboard 5 指标

## 验证

- [ ] 补偿成功率 ≥ 99.9%
- [ ] Saga P99 完成时间 < 5s
- [ ] 24h 压测无孤儿 Saga

## 参考

- [docs/08-架构优化v1.md §30](../../docs/08-架构优化v1.md)
- [docs/13-Saga-DSL-RFC.md §7](../../docs/13-Saga-DSL-RFC.md)
