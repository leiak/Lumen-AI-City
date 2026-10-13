# admin-portal

> **职责**：运营后台（NPC / Saga / BT 编辑器 / Saga DSL IDE / 创作者市场）
>
> **技术栈**：Next.js 15 + React 19 + TailwindCSS + React Flow + Monaco Editor

## 端口

`8081`

## 关键模块

| 路由 | 模块 | 文档 |
|---|---|---|
| `/` | 总览 | - |
| `/npc` | NPC 管理 | [02-NPC](../../docs/02-NPC人设与剧本.md) |
| `/saga` | Saga Dashboard | [§32.7](../../docs/08-架构优化v1.md) |
| `/admin/agent-templates` | Agent NPC 模板审核 | [4.0-M3](../../docs/4.0-M3-plan.md) |
| `/saga-viz` | Saga 可视化编辑器 | [4.0-M3](../../docs/4.0-M3-plan.md) |
| `/bt-editor` | 行为树编辑器 | [12-BT编辑器PRD](../../docs/12-BT编辑器PRD.md) |
| `/saga-dsl-ide` | Saga DSL IDE | [13-Saga-DSL-RFC](../../docs/13-Saga-DSL-RFC.md) |
| `/marketplace` | 创作者市场 | [§23](../../docs/07-MVP与ADR.md) |
