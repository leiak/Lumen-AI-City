# aicity-sdk (Python)

> **职责**：第三方 Agent 接入 AI City 联邦的 Python SDK
>
> **关键文档**：[docs/06-A2A协议.md §20.17](../../docs/06-A2A协议.md)

## 安装

```bash
pip install aicity-sdk
```

## 用法

```python
import asyncio
from aicity_sdk import A2AClient, AgentCard

async def main():
    client = A2AClient(
        gateway_url="https://aicity.example.com/a2a",
        api_key="sk-xxx",
    )

    # 1. 注册 Agent Card
    card = AgentCard(
        agent_id="my_agent_001",
        name="My Agent",
        url="https://my-agent.example.com",
        provider="openclaw",
        capabilities=["dialogue", "search"],
    )
    await client.register_card(card.model_dump())

    # 2. 发现联邦内的其他 Agent
    peers = await client.discover("dialogue")

    # 3. 发送消息
    await client.send_message(to_agent_id="npc_wang_boss_001", payload={"hello": "world"})

    await client.close()

asyncio.run(main())
```
