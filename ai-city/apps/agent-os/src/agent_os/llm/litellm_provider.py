# ai-city/apps/agent-os/src/agent_os/llm/litellm_provider.py
import os
import litellm
from .base import LLMProvider, LLMRequest, LLMResponse

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

    async def stream(self, req: LLMRequest):
        # TODO(week 2): 流式输出
        yield {"text": "[stream-stub]", "finish_reason": "stop"}
