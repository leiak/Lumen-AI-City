"""LiteLLM provider — 真 LLM 流式 + chat-completion 调用。

Spec ref: docs/superpowers/specs/2026-10-05-2.0-stage2-stream-emotion-design.md §1
"""
import inspect
import os
import time
from typing import AsyncIterator, Any

import litellm

from .base import LLMProvider, LLMRequest, LLMResponse
from .types import LLMRequest as StreamLLMRequest


class LiteLLMProvider(LLMProvider):
    def __init__(self, model: str = None):
        self.model = model or os.getenv("LLM_MODEL", "claude-sonnet-4-6")
        self.api_key = os.getenv("ANTHROPIC_API_KEY", "")

    async def complete(self, req: LLMRequest) -> LLMResponse:
        response = await litellm.acompletion(
            model=self.model,
            api_key=self.api_key,
            messages=[{"role": "system", "content": req.system_prompt}] + req.messages,
            max_tokens=req.max_tokens,
            temperature=req.temperature,
        )
        usage = response.usage
        return LLMResponse(
            text=response.choices[0].message.content,
            input_tokens=usage.prompt_tokens,
            output_tokens=usage.completion_tokens,
            finish_reason=response.choices[0].finish_reason,
        )

    async def stream(self, req: StreamLLMRequest) -> AsyncIterator[dict[str, Any]]:
        """真 LiteLLM 流式调用，逐 token yield ``{text, finish_reason}``。

        使用 ``llm.types.LLMRequest``（扁平 prompt + model），
        与 ``complete()`` 的 chat-completion 形态 ``llm.base.LLMRequest`` 区分。

        Spec ref: docs/superpowers/specs/2026-10-05-2.0-stage2-stream-emotion-design.md §1
        """
        response = litellm.acompletion(
            model=req.model,
            api_key=self.api_key,
            messages=[{"role": "user", "content": req.prompt}],
            max_tokens=req.max_tokens,
            temperature=req.temperature,
            stream=True,
        )
        # NOTE: real litellm.acompletion(stream=True) always returns a coroutine
        # (per https://docs.litellm.ai/docs/completion/stream). We also accept
        # async iterators directly for mock-friendliness in unit tests.
        if inspect.iscoroutine(response):
            response = await response
        async for chunk in response:
            if not chunk.choices:
                continue  # Skip empty choices (edge case)
            delta = chunk.choices[0].delta
            content = getattr(delta, "content", None)
            text = content if isinstance(content, str) else ""
            finish = chunk.choices[0].finish_reason
            yield {"text": text, "finish_reason": finish}
