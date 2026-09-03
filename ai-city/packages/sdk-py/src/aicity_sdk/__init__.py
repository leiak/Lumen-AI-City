"""AI City Python SDK - 第三方 Agent 接入。

详细设计见 docs/06-A2A协议.md §20.17。
"""
from aicity_sdk.client import A2AClient
from aicity_sdk.agent_card import AgentCard

__version__ = "0.1.0"
__all__ = ["A2AClient", "AgentCard"]
