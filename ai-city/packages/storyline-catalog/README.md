# packages/storyline-catalog

> **职责**：城市剧本模板库
>
> **关键文档**：[docs/02-NPC人设与剧本.md §16](../../docs/02-NPC人设与剧本.md)

## 文件清单

| 文件 | 剧本 | 类型 |
|---|---|---|
| `tavern_meeting.json` | 酒馆聚会 | 日常可重复 |
| `new_year_celebration.json` | 新年庆典 | 节日 |
| `...` | ...（共 5 个） | ... |

## 字段说明

- `id` - 唯一标识
- `trigger` - 触发条件（玩家进入 / 日历 / 关系度）
- `actors` - 参与的 NPC
- `steps` / `phases` - 步骤或阶段
- `compensations` - 失败补偿（Saga 集成）
- `success_criteria` - 成功条件

## 加载

由 `saga-orchestrator` 启动时拉取，运行时按需调用。
