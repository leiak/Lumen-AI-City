# agent-os

> **职责**：AI Agent 五模块循环运行时（Perception / Planning / Action / Reflection / Memory）
>
> **语言**：Python 3.12 + FastAPI
>
> **关键文档**：[docs/05-Agent-OS.md](../../docs/05-Agent-OS.md) / [docs/11-技术细节与玩法模式.md §B.1](../../docs/11-技术细节与玩法模式.md)

## 模块

| 模块 | 文件 | 说明 |
|---|---|---|
| Loop | `loop.py` | Tick + Event 混合驱动主循环 |
| Perception | `perception.py` | 视野内实体收集 |
| Planning | `planning.py` | 行为树 + LLM 升级 |
| LLM | `llm.py` | LiteLLM + 多 Provider fallback |
| LOD | `lod.py` | L0/L1/L2 三档决策降级 |
| Logger | `logger.py` | 单行 JSON 决策日志（§47） |
| App | `app.py` | FastAPI 入口 |

## 本地启动

```bash
uv pip install -e .
uvicorn agent_os.app:app --reload
```

## 端口

`8000`
