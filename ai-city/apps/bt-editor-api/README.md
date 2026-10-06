# bt-editor-api

> **职责**：BT 编辑器后端（校验 / 沙箱模拟 / 版本管理 / PG 持久化）
>
> **关键文档**：[docs/12-BT编辑器PRD.md](../../docs/12-BT编辑器PRD.md) §8 / §10
> **阶段**：Phase C.2（BT persistence + bt-editor-api 真后端）

## 端口

`8004`

## PG Schema

`db/migrations/pg-schema-2.0-bt-editor.sql` 创建 ``bt_tree`` 表（id, npc_id, name,
tree_json JSONB, version, created_at, updated_at）+ BEFORE UPDATE 触发器自动
bump version / updated_at。idempotent — 可重复执行。

## Endpoints

base path = ``/api/v1/bt``

| 方法 | 路径 | 说明 | 状态码 |
|---|---|---|---|
| GET    | ``/{npc_id}``                | 列出某 NPC 的全部 BT 树（按 updated_at 倒序） | 200 |
| GET    | ``/{npc_id}/{tree_name}``    | 取一棵树 | 200 / 404 |
| POST   | ``/{npc_id}/{tree_name}``    | upsert（带校验 + 深度/节点数限制） | 200 / 400 / 422 |
| POST   | ``/{npc_id}/{tree_name}/simulate`` | dry-run tick（带 trace log） | 200 / 400 |

### POST upsert 请求体

```json
{ "tree_json": { "id": "root", "type": "sequence", "children": [...] } }
```

返回：

```json
{
  "npc_id": "npc_alpha",
  "name": "greet_player",
  "tree_json": {...},
  "version": 2,
  "created_at": "2026-10-06T12:34:56+00:00",
  "updated_at": "2026-10-06T12:35:01+00:00"
}
```

### POST simulate 请求体

```json
{
  "tree_json": {...},
  "state": {
    "player_position": [0, 0],
    "npc_state": {"ready": true},
    "time_of_day": "noon",
    "max_ticks": 100
  },
  "tick_limit": 100
}
```

返回：

```json
{
  "status": "success",
  "trace": [{"node_id": "root", "status": "success", "tick_count": 4}],
  "final_state": {"player_position": [0, 0], "npc_state": {...}, ...}
}
```

## 错误码

| HTTP | detail.code | 触发 |
|---|---|---|
| 400 | `R_019` BT_PARSE_FAIL | tree_json 形状非法（缺 `type`、未知 discriminator、Pydantic ValidationError） |
| 422 | `R_019` BT_PARSE_FAIL | 树深 > `BT_TREE_DEPTH_LIMIT`（默认 10）或节点数 > `BT_TREE_NODE_LIMIT`（默认 50） |
| 404 | — | GET 找不到对应 (npc_id, tree_name) |
| 500 | `R_020` BT_EVAL_FAIL | （预留）simulate 内部 eval 失败 |

## 环境变量

| 变量 | 默认 | 说明 |
|---|---|---|
| `DATABASE_URL`        | `postgresql://aicity:aicity@localhost:5432/aicity` | asyncpg pool DSN |
| `BT_TREE_DEPTH_LIMIT` | `10` | 单棵树最大嵌套层数（POST 校验） |
| `BT_TREE_NODE_LIMIT`  | `50` | 单棵树最大节点数（POST 校验） |

## 验证复用

Phase C.2 端点的校验 / simulate 都直接调用 ``apps/agent-os/src/agent_os/bt/``
（C.1 GA）：

* ``load_tree(json_dict)`` — Pydantic discriminated union 校验（7 类节点）
* ``evaluator.tick(tree, state)`` — simulate 的实际执行

零业务校验逻辑重复；BT shape 改一处，editor API 自动同步。

## 测试

```
cd apps/bt-editor-api
uv run python -m pytest tests/
```

24 tests pass（13 endpoint + 11 db helpers）。不依赖 live PG — 用
``unittest.mock.AsyncMock`` 注入 asyncpg pool。
