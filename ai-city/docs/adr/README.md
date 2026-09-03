# Architecture Decision Records (ADR)

> 重大架构决策的可追溯记录。模板见 [`template.md`](template.md)。
>
> 详细设计见 [`docs/07-MVP与ADR.md §22.8`](../../docs/07-MVP与ADR.md)

## 已采纳的 ADR

| 编号 | 标题 | 状态 | 日期 |
|---|---|---|---|
| [ADR-0001](0001-use-python-for-agent-os.md) | Agent OS 使用 Python + FastAPI | Accepted | 2026-09-03 |
| [ADR-0002](0002-use-rust-for-world-engine.md) | World Engine 使用 Rust | Accepted | 2026-09-03 |
| [ADR-0003](0003-use-monorepo.md) | 采用 Monorepo + Microdeploy | Accepted | 2026-09-03 |
| [ADR-0004](0004-use-kafka-for-saga.md) | Saga 采用 Kafka + Orchestrator/Worker | Accepted | 2026-09-03 |
| [ADR-0005](0005-use-claude-sonnet-as-primary-llm.md) | 主 LLM 选 Claude Sonnet 4.6 | Accepted | 2026-09-03 |

## 待决策（候选）

- [ ] Saga DSL 是否替代 YAML 编排（见 §13）
- [ ] 客户端是否使用 WebGPU 渲染（vs MapLibre）
- [ ] 跨城联邦走专线 vs 公网
- [ ] 自托管 LLM 模型选型（国产替代）
- [ ] 创作者市场支付通道（支付宝 / Stripe / 加密货币）

## 评审节奏

- 每 2 周 1 次 ADR 评审会（周一上午）
- 任何 ADR 变更必须：
  1. 修改本文档
  2. 在 PR 中 @tech-lead 审批
  3. 影响下游时同步通知相关 owner

## 维护原则

> **"应该没问题"的判断，必须在故障注入和压测中被证明没问题。**
> **任何重大决策必进 ADR，无 ADR 不上线。**
