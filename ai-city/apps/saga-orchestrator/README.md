# saga-orchestrator

> **职责**：分布式事务编排（Step + Compensation + 状态机）
>
> **关键文档**：[docs/08-架构优化v1.md §30](../../docs/08-架构优化v1.md) / [docs/13-Saga-DSL-RFC.md §7](../../docs/13-Saga-DSL-RFC.md) / [docs/08-架构优化v1.md §32.7](../../docs/08-架构优化v1.md)（Dashboard）

## 端口

`8002`

## 关键 API

- `POST /v1/sagas` - 启动 Saga
- `GET /v1/sagas/{id}` - 查询状态
- `GET /metrics/saga` - Dashboard 指标（§32.7）
