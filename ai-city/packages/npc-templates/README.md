# packages/npc-templates

> **职责**：NPC 模板（OCEAN 五维人格 + 背景故事 + 行为树引用 + LLM prompt hints）
>
> **关键文档**：[docs/02-NPC人设与剧本.md §15](../../docs/02-NPC人设与剧本.md)

## 文件清单

| 文件 | NPC | OCEAN 简述 |
|---|---|---|
| `wang_boss.yaml` | 王老板（酒馆） | O35 C70 E55 A85 N40 |
| `lihua.yaml` | 李华（邻居） | O60 C75 E60 A65 N30 |
| `...` | ...（共 20 个） | ... |

## Schema

见 `OCEAN-schema.json`

## 加载流程

```python
import yaml
import json
import jsonschema

with open("OCEAN-schema.json") as f:
    schema = json.load(f)

with open("wang_boss.yaml") as f:
    npc = yaml.safe_load(f)

jsonschema.validate(npc, schema)
# → 加载到数据库 / Agent OS
```

## 版本管理

- 用 Git 版本化
- 修改 OCEAN 维度时记录 changelog
- 新增 NPC 走 PR 流程，至少 1 名策划 + 1 名 Tech Lead 审批
