# ai-city/apps/agent-os/tests/llm/test_claude_real.py
import os
import pytest
from agent_os.llm.litellm_provider import LiteLLMProvider
from agent_os.llm.base import LLMRequest
from agent_os.llm.types import LLMRequest as StreamLLMRequest

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


@pytest.mark.skipif(
    not os.getenv("ANTHROPIC_API_KEY"),
    reason="需要 ANTHROPIC_API_KEY",
)
@pytest.mark.asyncio
async def test_real_stream_yields_tokens(request):
    """LiteLLMProvider.stream() 真流式 token-by-token 测试（需 --run-real-llm + API key）。"""
    if not request.config.getoption("--run-real-llm"):
        pytest.skip("需要 --run-real-llm")
    import asyncio

    provider = LiteLLMProvider(model="claude-sonnet-4-6")
    req = StreamLLMRequest(
        prompt=(
            "<system>你是王老板。</system>\n"
            "<user>你好</user>\n"
            "请简短回答"
        ),
        model="claude-sonnet-4-6",
        max_tokens=128,
    )

    chunks = []
    async for chunk in provider.stream(req):
        chunks.append(chunk)
        if chunk.get("finish_reason") == "stop":
            break

    assert len(chunks) >= 3
    full = "".join(c["text"] for c in chunks)
    assert len(full) > 0
