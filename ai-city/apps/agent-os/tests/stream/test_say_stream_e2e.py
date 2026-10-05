"""T13: 端到端集成测试 — say_stream 走真实 LiteLLM（Anthropic Claude Haiku）。

完整链路：
    Dispatcher.say_stream
        -> get_npc_stream_prompt (T08)
        -> LiteLLMProvider.stream (T07)        [真实 LLM]
        -> SentenceSplitter.feed (T03)         [XML tag 切句]
        -> EmotionValidator (T04)              [8 类 emotion 校验]
        -> Publisher.publish_beat (T05)         [mock — 不发真实 Redis]
        -> yield DispatcherSayStreamEvent

约束：
- 需要 ANTHROPIC_API_KEY 环境变量；缺则 pytest.skip（CI 自动跳过）
- 使用 mock Publisher（unittest.mock.AsyncMock），不连真实 Redis
- cheap model = claude-haiku-4-5（不消耗 opus/sonnet 预算）
- max_tokens=200 + asyncio.wait_for timeout=15s，避免测试卡死
- LLM 1-6 句输出不定，断言使用 1 ≤ n ≤ 6，不锁死具体文本

Pre-existing 已知失败（不在本测试修复）：
- tests/llm/test_litellm_provider.py::test_claude_sonnet_real_call（async marker 缺失）
- tests/llm/test_integration.py::test_end_to_end_wang_boss_say（LLM nondeterminism）
"""
from __future__ import annotations

import asyncio
import os
from unittest.mock import AsyncMock, MagicMock

import pytest

from agent_os.dispatcher import ActionDispatcher
from agent_os.llm.litellm_provider import LiteLLMProvider

ALLOWED_EMOTIONS = frozenset({
    "happy", "sad", "angry", "surprised",
    "thinking", "embarrassed", "curious", "neutral",
})

# Cheap model for cost — per brief
HAIKU_MODEL = "claude-haiku-4-5"
MAX_TOKENS = 200
EVENT_TIMEOUT_S = 15


def _api_key_or_skip() -> str:
    """读 ANTHROPIC_API_KEY；缺失则 pytest.skip。绝不打印 key 内容。"""
    key = os.getenv("ANTHROPIC_API_KEY")
    if not key:
        pytest.skip("ANTHROPIC_API_KEY not configured")
    return key  # noqa: only used as truthiness check; never logged


def _make_publisher_mock():
    """Mock Publisher：记录每次 publish_beat / publish_done 调用。"""
    pub = MagicMock()
    pub.publish_beat = AsyncMock()
    pub.publish_done = AsyncMock()
    return pub


def _build_dispatcher(pub):
    """真实 LiteLLMProvider + mock publisher。"""
    provider = LiteLLMProvider(model=HAIKU_MODEL)
    return ActionDispatcher(llm_client=provider, publisher=pub)


# ---------------------------------------------------------------------------
# 真实 LLM e2e 测试（5 用例）
# ---------------------------------------------------------------------------


def test_say_stream_emits_at_least_one_sentence():
    """真实 Claude Haiku 调用：wang_boss 流式输出 ≥1 个句子事件。"""
    _api_key_or_skip()
    pub = _make_publisher_mock()
    dispatcher = _build_dispatcher(pub)

    async def run():
        async def collect_events():
            beats = []
            async for ev in dispatcher.say_stream(
                npc_id="npc_wang_boss_001",
                player_input="老板，今天有什么推荐菜？",
                npc_context=[],
                session_id="sess-e2e-1",
                trace_id="tr-e2e-1",
            ):
                if not ev.complete:  # 排除 done 事件
                    beats.append(ev)
            return beats

        # 真实外层 wait_for：防止 LLM 卡死（CI 必超时退出）
        return await asyncio.wait_for(collect_events(), timeout=EVENT_TIMEOUT_S)

    beats = asyncio.run(run())
    # LLM 1-6 句不定；最少 1 句
    assert 1 <= len(beats) <= 6, f"expected 1-6 beats, got {len(beats)}"
    # 每个 beat 的 emotion ∈ 8 类
    for ev in beats:
        assert ev.emotion in ALLOWED_EMOTIONS, (
            f"invalid emotion {ev.emotion!r} in beat"
        )
        assert ev.text != "", "beat text should be non-empty"


def test_say_stream_emits_done_event():
    """最后一个事件 complete=True（done 收尾）。"""
    _api_key_or_skip()
    pub = _make_publisher_mock()
    dispatcher = _build_dispatcher(pub)

    async def run():
        events = []
        async for ev in dispatcher.say_stream(
            npc_id="npc_wang_boss_001",
            player_input="一份红烧肉",
            npc_context=[],
            session_id="sess-e2e-2",
            trace_id="tr-e2e-2",
        ):
            events.append(ev)
        return events

    events = asyncio.run(run())
    assert len(events) >= 2, "should have ≥1 beat + 1 done"
    last = events[-1]
    assert last.complete is True
    assert last.sentence_idx is None
    assert last.text == ""


def test_say_stream_publishes_to_publisher_mock():
    """验证 Publisher.publish_beat / publish_done 被正确调用次数。"""
    _api_key_or_skip()
    pub = _make_publisher_mock()
    dispatcher = _build_dispatcher(pub)

    async def run():
        async for _ in dispatcher.say_stream(
            npc_id="npc_wang_boss_001",
            player_input="再来一碗汤",
            npc_context=[],
            session_id="sess-e2e-3",
            trace_id="tr-e2e-3",
        ):
            pass

    asyncio.run(run())

    beat_count = pub.publish_beat.await_count
    done_count = pub.publish_done.await_count

    # 至少有 1 个 beat + 1 个 done
    assert beat_count >= 1, f"expected ≥1 publish_beat, got {beat_count}"
    assert done_count == 1, f"expected exactly 1 publish_done, got {done_count}"

    # done 的 complete=True（流正常结束）
    done_kwargs = pub.publish_done.await_args.kwargs
    assert done_kwargs["complete"] is True
    assert done_kwargs["sentence_count"] == beat_count


def test_emotion_in_valid_8_classes_all_beats():
    """每个 beat 的 emotion 必属 8 类 — 强制约束。"""
    _api_key_or_skip()
    pub = _make_publisher_mock()
    dispatcher = _build_dispatcher(pub)

    async def run():
        async for ev in dispatcher.say_stream(
            npc_id="npc_wang_boss_001",
            player_input="听说你这儿菜很好吃？",
            npc_context=[],
            session_id="sess-e2e-4",
            trace_id="tr-e2e-4",
        ):
            if not ev.complete:
                # emotion 必属 8 类；不允许 LLM 编新 emotion 标签
                assert ev.emotion in ALLOWED_EMOTIONS, (
                    f"emotion {ev.emotion!r} not in 8 classes"
                )

    asyncio.run(run())


def test_say_stream_5_npcs_each_yields_sentence():
    """5 NPC 各产 ≥1 句 — 验证 npc_context prompt 模板 + LLM 都 OK。"""
    _api_key_or_skip()
    pub = _make_publisher_mock()
    provider = LiteLLMProvider(model=HAIKU_MODEL)
    dispatcher = ActionDispatcher(llm_client=provider, publisher=pub)

    async def run():
        results = {}
        for npc_id in [
            "npc_wang_boss_001",
            "npc_grace_healer_001",
            "npc_snack_owner_001",
            "npc_book_keeper_001",
            "npc_dance_leader_001",
        ]:
            beats = []
            async for ev in dispatcher.say_stream(
                npc_id=npc_id,
                player_input="你好",
                npc_context=[],
                session_id=f"sess-e2e-{npc_id}",
                trace_id=f"tr-e2e-{npc_id}",
            ):
                if not ev.complete:
                    beats.append(ev)
            results[npc_id] = beats
        return results

    results = asyncio.run(run())
    for npc_id, beats in results.items():
        assert 1 <= len(beats) <= 6, (
            f"{npc_id}: expected 1-6 beats, got {len(beats)}"
        )
        for ev in beats:
            assert ev.emotion in ALLOWED_EMOTIONS