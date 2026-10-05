"""端到端集成：LLM mock → dispatcher.say_stream() → Publisher mock → 节拍包验证。"""
import asyncio
from unittest.mock import MagicMock, AsyncMock
import pytest

from agent_os.dispatcher import ActionDispatcher


@pytest.fixture
def integration_setup():
    llm = MagicMock()
    pub = MagicMock()
    pub.publish_beat = AsyncMock()
    pub.publish_done = AsyncMock()
    dispatcher = ActionDispatcher(llm_client=llm, publisher=pub)
    return dispatcher, pub


def test_full_pipeline_3_events(integration_setup):
    """完整流式管道：2 句 + 1 done。"""
    dispatcher, pub = integration_setup

    async def fake_stream(req):
        yield {"text": "<emotion=happy>来了您嘞！</emotion>", "finish_reason": None}
        yield {"text": "<emotion=neutral>几位？</emotion>", "finish_reason": None}
        yield {"text": "<end>", "finish_reason": "stop"}

    dispatcher.llm_client.stream = fake_stream

    async def run():
        events = []
        async for ev in dispatcher.say_stream(
            npc_id="npc_wang_boss_001",
            player_input="点菜",
            npc_context=[],
            session_id="sess-int",
            trace_id="tr-int",
        ):
            events.append(ev)
        return events

    events = asyncio.run(run())
    assert len(events) == 3
    assert pub.publish_beat.await_count == 2
    assert pub.publish_done.await_count == 1


def test_emotion_validation_integration(integration_setup):
    """LLM 输出无效 emotion → 降级 neutral。"""
    dispatcher, pub = integration_setup

    async def fake_stream(req):
        yield {"text": "<emotion=lol>异常情绪。</emotion>", "finish_reason": None}
        yield {"text": "<end>", "finish_reason": "stop"}

    dispatcher.llm_client.stream = fake_stream

    async def run():
        async for _ in dispatcher.say_stream(
            npc_id="npc_test", player_input="x", npc_context=[],
            session_id="sess-x", trace_id="tr-x",
        ):
            pass

    asyncio.run(run())
    # 第一次 publish_beat 应是降级 neutral
    call_args = pub.publish_beat.await_args_list[0]
    assert call_args.kwargs["emotion"] == "neutral"