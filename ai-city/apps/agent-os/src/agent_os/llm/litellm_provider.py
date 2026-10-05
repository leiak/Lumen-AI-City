# ai-city/apps/agent-os/src/agent_os/llm/litellm_provider.py
import inspect
import os
import litellm
from .base import LLMProvider, LLMRequest, LLMResponse
from .types import LLMRequest as StreamLLMRequest


class LiteLLMProvider(LLMProvider):
    def __init__(self, model: str = None):
        self.model = model or os.getenv("LLM_MODEL", "claude-sonnet-4-6")
        self.api_key = os.getenv("ANTHROPIC_API_KEY", "")

    async def complete(self, req: LLMRequest) -> LLMResponse:
        import time
        start = time.time()
        response = await litellm.acompletion(
            model=self.model,
            api_key=self.api_key,
            messages=[{"role": "system", "content": req.system_prompt}] + req.messages,
            max_tokens=req.max_tokens,
            temperature=req.temperature,
        )
        elapsed_ms = int((time.time() - start) * 1000)
        usage = response.usage
        return LLMResponse(
            text=response.choices[0].message.content,
            input_tokens=usage.prompt_tokens,
            output_tokens=usage.completion_tokens,
            finish_reason=response.choices[0].finish_reason,
        )

    async def stream(self, req: StreamLLMRequest):
        """真 LiteLLM 流式调用，逐 token yield ``{text, finish_reason}``。

        使用 ``llm.types.LLMRequest``（扁平 prompt + model），
        与 ``complete()`` 的 chat-completion 形态 ``llm.base.LLMRequest`` 区分。

        Spec ref: docs/superpowers/specs/2026-10-05-2.0-stage2-stream-emotion-design.md §1
        """
        # Note: litellm.acompletion(stream=True) returns either a coroutine
        # (real lib) or an async iterator directly (some mocks/wrappers).
        # Await only if it's actually a coroutine.
        response = litellm.acompletion(
            model=req.model,
            api_key=self.api_key,
            messages=[{"role": "user", "content": req.prompt}],
            max_tokens=req.max_tokens,
            temperature=req.temperature,
            stream=True,
        )
        if inspect.iscoroutine(response):
            response = await response
        async for chunk in response:
            delta = chunk.choices[0].delta
            text = delta.content or ""
            finish = chunk.choices[0].finish_reason
            yield {"text": text, "finish_reason": finish}
