# ai-city/apps/agent-os/tests/llm/test_claude_real.py
import os
import pytest
from agent_os.llm.litellm_provider import LiteLLMProvider
from agent_os.llm.base import LLMRequest

@pytest.mark.skipif(not os.getenv("ANTHROPIC_API_KEY"), reason="no API key")
async def test_claude_sonnet_real_call():
    provider = LiteLLMProvider(model="claude-sonnet-4-6")
    req = LLMRequest(
        system_prompt="你是一个简洁的 NPC。",
        messages=[{"role": "user", "content": "用一句话自我介绍"}],
        max_tokens=50,
    )
    resp = await provider.complete(req)
    assert len(resp.text) > 0
    assert resp.input_tokens > 0
    assert resp.output_tokens > 0
    assert resp.finish_reason == "stop"
