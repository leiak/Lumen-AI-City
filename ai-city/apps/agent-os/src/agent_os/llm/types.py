"""LLMRequest 数据类（T07 已完成 LiteLLM 集成）。

用于单轮 XML-tag prompt（如 ``say_stream`` 流式输出）。
多轮 chat-completion 请用 ``llm.base.LLMRequest``。

两者并存原因（spec §1）：
- ``llm.base.LLMRequest`` 是 chat-completion 形态（system_prompt + messages），
  给 ``say_with_llm`` 多轮历史用。
- ``llm.types.LLMRequest``（本模块）是扁平 prompt 形态（prompt + model），
  给 ``say_stream`` 单轮 XML-tag 流式输出用。
"""
from __future__ import annotations

from dataclasses import dataclass


@dataclass
class LLMRequest:
    """单轮 prompt 请求（流式 XML tag 输出）。"""

    prompt: str
    model: str = "claude-sonnet-4-6"
    max_tokens: int = 512
    temperature: float = 0.7
