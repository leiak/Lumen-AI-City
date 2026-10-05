"""2.0 dispatcher.say_with_llm 集成 LLM + 短路词 + chat_turn 测试。

也覆盖 dispatcher.say_stream() 流式入口（T06）。
"""

import asyncio
from unittest.mock import AsyncMock, MagicMock

import pytest

from agent_os.dispatcher import ActionDispatcher
from agent_os.llm.litellm_provider import LiteLLMProvider


@pytest.mark.asyncio
async def test_dispatcher_say_with_llm():
    """真实 LLM 调用 → 返回非空文本（需要 ANTHROPIC_API_KEY）。"""
    llm = LiteLLMProvider(model="claude-sonnet-4-6")
    dispatcher = ActionDispatcher(llm_client=llm)
    result = await dispatcher.say_with_llm(
        npc_id="npc_a_wang_boss",
        player_input="今天有什么好吃的？",
        npc_context=[],
        chat_turn=1,
    )
    assert result.npc_id == "npc_a_wang_boss"
    assert len(result.text) > 0
    assert result.trace_id != ""


@pytest.mark.asyncio
async def test_dispatcher_say_chat_turn_6_close():
    """chat_turn=6 → 强制收尾（无 LLM 调用）。"""
    llm = LiteLLMProvider(model="claude-sonnet-4-6")
    dispatcher = ActionDispatcher(llm_client=llm)
    result = await dispatcher.say_with_llm(
        npc_id="npc_a_wang_boss",
        player_input="再聊一个",
        npc_context=[],
        chat_turn=6,  # 第 6 回合
    )
    assert result.text == "闲聊到此，下次再来吧！"
    assert result.trace_id != ""


# ---------------------------------------------------------------------------
# T06: dispatcher.say_stream() 流式入口测试
# ---------------------------------------------------------------------------


@pytest.fixture
def stream_dispatcher():
    """带 mock llm_client + mock publisher 的 dispatcher（不走真实 LLM）。"""
    llm = MagicMock()
    pub = MagicMock()
    pub.publish_beat = AsyncMock()
    pub.publish_done = AsyncMock()
    return ActionDispatcher(llm_client=llm, publisher=pub)


def test_say_stream_emits_beat_events(stream_dispatcher):
    """say_stream yield 句子节拍事件：2 句 beat + 1 done。"""

    async def fake_stream(req):
        # 3 token chunks: sentence1, sentence2, <end>
        yield {"text": "<emotion=happy>来了您嘞！</emotion>", "finish_reason": None}
        yield {"text": "<emotion=neutral>几位？</emotion>", "finish_reason": None}
        yield {"text": "<end>", "finish_reason": "stop"}

    stream_dispatcher.llm_client.stream = fake_stream

    async def run():
        events = []
        async for ev in stream_dispatcher.say_stream(
            npc_id="npc_wang_boss_001",
            player_input="点菜",
            npc_context=[],
            session_id="sess-test",
            trace_id="tr-test",
        ):
            events.append(ev)
        return events

    events = asyncio.run(run())
    assert len(events) == 3  # 2 句 beat + 1 done
    assert events[0].text == "来了您嘞！"
    assert events[0].emotion == "happy"
    assert events[0].sentence_idx == 0
    assert events[0].complete is False
    assert events[1].text == "几位？"
    assert events[1].emotion == "neutral"
    assert events[1].sentence_idx == 1
    assert events[2].complete is True
    assert events[2].sentence_idx is None  # done 事件无 sentence_idx


def test_say_stream_publishes_beat_and_done(stream_dispatcher):
    """say_stream 通过 publisher 发送每句 beat + done(complete=True)。"""

    async def fake_stream(req):
        yield {"text": "<emotion=happy>来了您嘞！</emotion>", "finish_reason": None}
        yield {"text": "<end>", "finish_reason": "stop"}

    stream_dispatcher.llm_client.stream = fake_stream

    async def run():
        async for _ in stream_dispatcher.say_stream(
            npc_id="npc_wang_boss_001",
            player_input="点菜",
            npc_context=[],
            session_id="sess-test",
            trace_id="tr-test",
        ):
            pass

    asyncio.run(run())
    # 1 个 beat + 1 个 done(complete=True)
    assert stream_dispatcher.publisher.publish_beat.await_count == 1
    assert stream_dispatcher.publisher.publish_done.await_count == 1
    done_kwargs = stream_dispatcher.publisher.publish_done.await_args.kwargs
    assert done_kwargs["complete"] is True
    assert done_kwargs["sentence_count"] == 1


def test_say_stream_on_exception_publishes_done_incomplete(stream_dispatcher):
    """LLM 流式异常 → 发 done(complete=False) + raise（R_011 fallback）。"""

    async def fake_stream(req):
        raise ConnectionError("LLM down")
        yield  # unreachable, makes this an async generator

    stream_dispatcher.llm_client.stream = fake_stream

    async def run():
        async for _ in stream_dispatcher.say_stream(
            npc_id="npc_wang_boss_001",
            player_input="点菜",
            npc_context=[],
            session_id="sess-test",
            trace_id="tr-test",
        ):
            pass

    with pytest.raises(ConnectionError):
        asyncio.run(run())
    # 异常时仍发 done(complete=False)
    assert stream_dispatcher.publisher.publish_done.await_count == 1
    done_kwargs = stream_dispatcher.publisher.publish_done.await_args.kwargs
    assert done_kwargs["complete"] is False


def test_say_stream_auto_generates_trace_id(stream_dispatcher):
    """trace_id=None → 内部生成 tr-xxx。"""

    async def fake_stream(req):
        yield {"text": "<end>", "finish_reason": "stop"}

    stream_dispatcher.llm_client.stream = fake_stream

    async def run():
        events = []
        async for ev in stream_dispatcher.say_stream(
            npc_id="npc_wang_boss_001",
            player_input="点菜",
            npc_context=[],
            session_id="sess-test",
            trace_id=None,
        ):
            events.append(ev)
        return events

    events = asyncio.run(run())
    assert events[-1].trace_id.startswith("tr-")
    assert len(events[-1].trace_id) > 3