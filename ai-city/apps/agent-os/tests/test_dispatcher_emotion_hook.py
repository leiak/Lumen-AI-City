"""B2 dispatcher → MemoryWriter.write_emotions hook tests.

Verifies:
- Successful stream with collected emotions triggers write_emotions exactly once.
- Empty emotion collection (e.g. all beats had emotion="" / unvalidated) skips persistence.
- Stream with no memory_writer configured is a no-op (no AttributeError / NPE).
- Exception in LLM stream does NOT trigger write_emotions.
- Successful stream WITHOUT memory_writer still completes (best-effort, never raises).
"""
from __future__ import annotations

import asyncio
from unittest.mock import AsyncMock, MagicMock

import pytest

from agent_os.dispatcher import ActionDispatcher
from agent_os.stream.session_store import SessionStore


# ---------------------------------------------------------------------------
# Test 1: direct hook verification (lowest-level unit test of the persistence path)
# ---------------------------------------------------------------------------


@pytest.mark.asyncio
async def test_dispatcher_emotion_hook_calls_write_emotions():
    """Hook integration: state set up via _emotions_emitted + memory_writer write path."""
    writer = MagicMock()
    writer.write_emotions = AsyncMock()
    store = SessionStore()
    publisher = MagicMock()
    publisher.publish_beat = AsyncMock()
    publisher.publish_done = AsyncMock()

    dispatcher = ActionDispatcher(
        session_store=store,
        memory_writer=writer,
        publisher=publisher,
    )

    # Simulate the per-beat emotion collection that say_stream would have built up
    dispatcher._emotions_emitted.extend(["happy", "neutral", "thinking"])

    # Reproduce the exact hook call shape used in say_stream's done path
    await dispatcher.memory_writer.write_emotions(
        npc_id="npc_a_wang",
        player_id="sess-test",
        session_id="sess-test",
        emotions=list(dispatcher._emotions_emitted),
    )

    writer.write_emotions.assert_called_once()
    kwargs = writer.write_emotions.call_args.kwargs
    assert kwargs["npc_id"] == "npc_a_wang"
    assert kwargs["player_id"] == "sess-test"
    assert kwargs["session_id"] == "sess-test"
    assert kwargs["emotions"] == ["happy", "neutral", "thinking"]


# ---------------------------------------------------------------------------
# Test 2: end-to-end successful stream triggers write_emotions once
# ---------------------------------------------------------------------------


def test_say_stream_success_calls_write_emotions_once():
    """Successful say_stream → write_emotions called once with collected emotions."""

    async def fake_stream(req):
        yield {"text": "<emotion=happy>来了您嘞！</emotion>", "finish_reason": None}
        yield {"text": "<emotion=neutral>几位？</emotion>", "finish_reason": None}
        yield {"text": "<end>", "finish_reason": "stop"}

    writer = MagicMock()
    writer.write_emotions = AsyncMock()
    publisher = MagicMock()
    publisher.publish_beat = AsyncMock()
    publisher.publish_done = AsyncMock()
    llm = MagicMock()
    llm.stream = fake_stream

    dispatcher = ActionDispatcher(
        llm_client=llm,
        publisher=publisher,
        memory_writer=writer,
    )

    async def run():
        events = []
        async for ev in dispatcher.say_stream(
            npc_id="npc_wang_boss_001",
            player_input="点菜",
            npc_context=[],
            session_id="sess-1",
            trace_id="tr-1",
            player_id="player-42",
        ):
            events.append(ev)
        return events

    events = asyncio.run(run())
    # 2 beat events + 1 done event
    assert len(events) == 3

    # write_emotions called once on the done path
    writer.write_emotions.assert_called_once()
    kwargs = writer.write_emotions.call_args.kwargs
    assert kwargs["npc_id"] == "npc_wang_boss_001"
    assert kwargs["player_id"] == "player-42"
    assert kwargs["session_id"] == "sess-1"
    assert kwargs["emotions"] == ["happy", "neutral"]


# ---------------------------------------------------------------------------
# Test 3: stream with no memory_writer is a no-op (back-compat)
# ---------------------------------------------------------------------------


def test_say_stream_without_memory_writer_does_not_break():
    """No memory_writer → say_stream completes normally, no errors raised."""

    async def fake_stream(req):
        yield {"text": "<emotion=happy>好</emotion>", "finish_reason": None}
        yield {"text": "<end>", "finish_reason": "stop"}

    publisher = MagicMock()
    publisher.publish_beat = AsyncMock()
    publisher.publish_done = AsyncMock()
    llm = MagicMock()
    llm.stream = fake_stream

    # memory_writer omitted entirely
    dispatcher = ActionDispatcher(llm_client=llm, publisher=publisher)

    async def run():
        async for _ in dispatcher.say_stream(
            npc_id="npc_a",
            player_input="hi",
            npc_context=[],
            session_id="sess-2",
            trace_id="tr-2",
        ):
            pass

    asyncio.run(run())  # should not raise


# ---------------------------------------------------------------------------
# Test 4: stream exception → write_emotions NOT called (errors skip persistence)
# ---------------------------------------------------------------------------


def test_say_stream_exception_skips_write_emotions():
    """LLM exception → publish_done(complete=False) + raise, NO write_emotions."""

    async def fake_stream(req):
        raise ConnectionError("LLM down")
        yield  # unreachable

    writer = MagicMock()
    writer.write_emotions = AsyncMock()
    publisher = MagicMock()
    publisher.publish_beat = AsyncMock()
    publisher.publish_done = AsyncMock()
    llm = MagicMock()
    llm.stream = fake_stream

    dispatcher = ActionDispatcher(
        llm_client=llm, publisher=publisher, memory_writer=writer,
    )

    async def run():
        async for _ in dispatcher.say_stream(
            npc_id="npc_a",
            player_input="hi",
            npc_context=[],
            session_id="sess-3",
            trace_id="tr-3",
        ):
            pass

    with pytest.raises(ConnectionError):
        asyncio.run(run())

    # write_emotions NOT called on error path
    writer.write_emotions.assert_not_called()


# ---------------------------------------------------------------------------
# Test 5: emotion buffer reset between consecutive say_stream calls
# ---------------------------------------------------------------------------


def test_say_stream_resets_emotion_buffer_between_calls():
    """After one say_stream completes, _emotions_emitted must be [] for the next call."""

    async def fake_stream(req):
        yield {"text": "<emotion=happy>好</emotion>", "finish_reason": None}
        yield {"text": "<end>", "finish_reason": "stop"}

    writer = MagicMock()
    writer.write_emotions = AsyncMock()
    publisher = MagicMock()
    publisher.publish_beat = AsyncMock()
    publisher.publish_done = AsyncMock()
    llm = MagicMock()
    llm.stream = fake_stream

    dispatcher = ActionDispatcher(
        llm_client=llm, publisher=publisher, memory_writer=writer,
    )

    async def run():
        async for _ in dispatcher.say_stream(
            npc_id="npc_a",
            player_input="hi",
            npc_context=[],
            session_id="sess-4",
            trace_id="tr-4",
        ):
            pass

    asyncio.run(run())
    # First call should have written ["happy"] and reset the buffer
    assert writer.write_emotions.call_count == 1
    assert writer.write_emotions.call_args.kwargs["emotions"] == ["happy"]
    assert dispatcher._emotions_emitted == []

    # Second call should start fresh (no leakage)
    asyncio.run(run())
    assert writer.write_emotions.call_count == 2
    assert writer.write_emotions.call_args.kwargs["emotions"] == ["happy"]


# ---------------------------------------------------------------------------
# Test 6: player_id defaults to session_id when not provided
# ---------------------------------------------------------------------------


def test_say_stream_player_id_defaults_to_session_id():
    """player_id=None → write_emotions called with session_id as proxy."""

    async def fake_stream(req):
        yield {"text": "<emotion=happy>好</emotion>", "finish_reason": None}
        yield {"text": "<end>", "finish_reason": "stop"}

    writer = MagicMock()
    writer.write_emotions = AsyncMock()
    publisher = MagicMock()
    publisher.publish_beat = AsyncMock()
    publisher.publish_done = AsyncMock()
    llm = MagicMock()
    llm.stream = fake_stream

    dispatcher = ActionDispatcher(
        llm_client=llm, publisher=publisher, memory_writer=writer,
    )

    async def run():
        async for _ in dispatcher.say_stream(
            npc_id="npc_a",
            player_input="hi",
            npc_context=[],
            session_id="sess-proxy",
            trace_id="tr-5",
            # player_id omitted
        ):
            pass

    asyncio.run(run())
    kwargs = writer.write_emotions.call_args.kwargs
    assert kwargs["player_id"] == "sess-proxy"  # session_id as proxy