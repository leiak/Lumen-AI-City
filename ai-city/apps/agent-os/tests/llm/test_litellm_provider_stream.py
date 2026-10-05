"""LiteLLMProvider.stream() 单元测试（mock LiteLLM，不需要 API key）。"""
import asyncio
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from agent_os.llm.litellm_provider import LiteLLMProvider
from agent_os.llm.types import LLMRequest


def _make_stream_chunk(text: str, finish_reason=None):
    """构造 LiteLLM streaming chunk mock（delta.content + finish_reason）。"""
    chunk = MagicMock()
    chunk.choices = [MagicMock()]
    chunk.choices[0].delta = MagicMock()
    chunk.choices[0].delta.content = text
    chunk.choices[0].finish_reason = finish_reason
    return chunk


def test_stream_yields_dicts():
    """stream() yield {text, finish_reason} 字典。"""
    fake_chunks = [
        _make_stream_chunk("你好", None),
        _make_stream_chunk("！", "stop"),
    ]

    async def fake_acompletion(*args, **kwargs):
        for c in fake_chunks:
            yield c

    with patch("agent_os.llm.litellm_provider.litellm.acompletion", fake_acompletion):
        provider = LiteLLMProvider()
        req = LLMRequest(prompt="hi", model="claude-sonnet-4-6")

        async def run():
            return [c async for c in provider.stream(req)]

        chunks = asyncio.run(run())

    assert len(chunks) == 2
    assert chunks[0] == {"text": "你好", "finish_reason": None}
    assert chunks[1] == {"text": "！", "finish_reason": "stop"}


def test_stream_handles_empty_content():
    """delta.content 为 None → text 默认为空字符串。"""
    chunk = MagicMock()
    chunk.choices = [MagicMock()]
    chunk.choices[0].delta = MagicMock()
    chunk.choices[0].delta.content = None
    chunk.choices[0].finish_reason = "stop"

    async def fake_acompletion(*args, **kwargs):
        yield chunk

    with patch("agent_os.llm.litellm_provider.litellm.acompletion", fake_acompletion):
        provider = LiteLLMProvider()
        req = LLMRequest(prompt="hi")

        async def run():
            return [c async for c in provider.stream(req)]

        chunks = asyncio.run(run())

    assert chunks == [{"text": "", "finish_reason": "stop"}]
