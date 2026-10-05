"""LLMRequest 数据类（T07 会扩展 LiteLLM 集成）。

与 ``llm.base.LLMRequest``（chat completion 风格）并存：本模块是
2.0 stage2 流式场景用的"prompt 字符串 + model"扁平形态，便于
T07 直接接入 LiteLLM 的 completion(stream=True) 入口。
"""
from __future__ import annotations

from dataclasses import dataclass


@dataclass
class LLMRequest:
    """流式 LLM 请求（扁平 prompt 形态）。"""

    prompt: str
    model: str = "claude-sonnet-4-6"
    max_tokens: int = 512
    temperature: float = 0.7
