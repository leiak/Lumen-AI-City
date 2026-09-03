# packages/saga-dsl

> **职责**：Saga DSL 解析器 / 编译器 / 运行时 / 内置函数
>
> **关键文档**：[docs/13-Saga-DSL-RFC.md](../../docs/13-Saga-DSL-RFC.md) 全文

## 模块

| 文件 | 角色 |
|---|---|
| `grammar.ebnf` | DSL 语法（EBNF） |
| `parser.py` | Lark parser |
| `compiler.py` | AST → CompiledSaga |
| `runtime.py` | 执行 + 补偿 |
| `builtins/` | 80+ 内置函数（§4.4） |

## 5 个内置函数类别

| 类别 | 数量 | 示例 |
|---|---|---|
| 时间 | 8 | `now()`, `add_seconds(t, 10)`, `is_daytime()` |
| 位置 | 7 | `distance(a, b)`, `is_in_tile(p, "tile_0_0")` |
| 物品 | 12 | `give_item(p, "key", 1)`, `has_item(p, "sword")` |
| 关系 | 9 | `relationship(a, b)`, `add_relationship(...)` |
| 玩家/NPC | 14 | `player_money(p)`, `npc_mood(n)` |
| Saga | 11 | `emit_event(...)`, `set_state(...)` |
| 记忆 | 6 | `remember(...)`, `recall(...)` |
| 货币 | 7 | `transfer(from, to, amount)` |
| 其他 | 6+ | ... |

## 用法

```python
from saga_dsl import parse, compile_saga, SagaRuntime

source = """
saga example {
    name: "test"
}

trigger {
    on "player_entered_tile"
}

step greet {
    do: npc_say(actor="npc_wang", content="welcome")
    timeout: 5s
    retries: 2
}
"""

ast = parse(source)
saga = compile_saga(ast)
runtime = SagaRuntime(saga, action_registry={...})
await runtime.run(ctx)
```
