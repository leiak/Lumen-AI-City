"""LLM provider 抽象包（base interface + LiteLLM provider）。

使用：
    from agent_os.llm import LiteLLMProvider
    from agent_os.llm import LLMRequest as ChatLLMRequest  # 多轮 chat-completion
    from agent_os.llm.types import LLMRequest as StreamLLMRequest  # 单轮流式

两个 LLMRequest 的区别：
- ``llm.base.LLMRequest`` (chat-completion: system_prompt + messages)
  → ``LiteLLMProvider.complete()`` + ``dispatcher.say_with_llm()``
- ``llm.types.LLMRequest`` (扁平 prompt + model)
  → ``LiteLLMProvider.stream()`` + ``dispatcher.say_stream()``
"""
from agent_os.llm.base import LLMProvider, LLMRequest, LLMResponse
from agent_os.llm.litellm_provider import LiteLLMProvider

__all__ = [
    "LLMProvider",
    "LLMRequest",
    "LLMResponse",
    "LiteLLMProvider",
]
