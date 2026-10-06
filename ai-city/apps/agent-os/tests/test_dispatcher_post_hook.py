"""W4.2: dispatcher post-hook → npc_sell_to_player integration tests.

Verifies the opt-in fire-and-forget behavior of ``schedule_post_hook``
and ``say_stream(..., enable_bt_post_hook=True, pending_purchase=...)``.

Three tests:

1. ``test_post_hook_disabled_returns_none`` — env kill-switch off → no task.
2. ``test_post_hook_no_pending_purchase_returns_none`` — no queued purchase
   → no task (avoids spinning up an empty coroutine).
4. ``test_post_hook_runs_npc_sell_to_player_action`` — env enabled + pending
   purchase → task scheduled, BT action invoked with queued kwargs.
5. ``test_say_stream_with_post_hook_dispatches_purchase`` — full say_stream()
   integration: mock LLM stream → mock publisher → mock BT registry; verify
   the post-hook runs after the done event without blocking the caller.
"""
from __future__ import annotations

import asyncio
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from agent_os.bt.state import BTState
from agent_os.dispatcher import (
    ActionDispatcher,
    schedule_post_hook,
)

# ---------------------------------------------------------------------------
# Unit tests for schedule_post_hook
# ---------------------------------------------------------------------------


@pytest.mark.asyncio
async def test_post_hook_disabled_returns_none():
    """BT_POST_HOOK_ENABLED=False → no task scheduled."""
    with patch("agent_os.dispatcher.BT_POST_HOOK_ENABLED", False):
        state = BTState(player_id="p1", pending_purchase={"product_id": 1})
        result = schedule_post_hook(state)
        assert result is None


@pytest.mark.asyncio
async def test_post_hook_no_pending_purchase_returns_none():
    """pending_purchase=None → no task scheduled (even if enabled)."""
    with patch("agent_os.dispatcher.BT_POST_HOOK_ENABLED", True):
        state = BTState(player_id="p1")
        result = schedule_post_hook(state)
        assert result is None


@pytest.mark.asyncio
async def test_post_hook_runs_npc_sell_to_player_action():
    """Pending purchase + enabled → schedules npc_sell_to_player task."""
    state = BTState(
        player_id="p1",
        pending_purchase={"product_id": 42, "currency": "gold"},
    )
    with (
        patch("agent_os.dispatcher.BT_POST_HOOK_ENABLED", True),
        patch("agent_os.bt.actions.ACTION_REGISTRY") as mock_reg,
    ):
        mock_action = MagicMock()
        # npc_sell_to_player returns a Status enum, use MagicMock for simplicity
        mock_action.return_value.name = "SUCCESS"
        mock_reg.get.return_value = mock_action

        task = schedule_post_hook(state)
        assert task is not None
        await task  # let the background coroutine complete

        mock_reg.get.assert_called_with("npc_sell_to_player")
        mock_action.assert_called_once_with(
            state=state, product_id=42, currency="gold",
        )


# ---------------------------------------------------------------------------
# say_stream() integration test
# ---------------------------------------------------------------------------


@pytest.mark.asyncio
async def test_say_stream_with_post_hook_dispatches_purchase():
    """Full say_stream() path: post-hook runs after the done event without awaiting."""
    llm = MagicMock()
    pub = MagicMock()
    pub.publish_beat = AsyncMock()
    pub.publish_done = AsyncMock()

    dispatcher = ActionDispatcher(llm_client=llm, publisher=pub)

    async def fake_stream(req):
        yield {"text": "<emotion=happy>来了您嘞！</emotion>", "finish_reason": None}
        yield {"text": "<end>", "finish_reason": "stop"}

    dispatcher.llm_client.stream = fake_stream

    # Track which action kwargs ran. The real npc_sell_to_player is sync,
    # so the fake must be sync too — _run_post_hook invokes it directly
    # without awaiting (the asyncio.create_task wrap is the only async hop).
    captured_kwargs: dict = {}
    completed_event = asyncio.Event()

    def fake_npc_sell_to_player(*, state, product_id, currency):
        captured_kwargs["product_id"] = product_id
        captured_kwargs["currency"] = currency
        captured_kwargs["player_id"] = state.player_id
        # Use a thread-safe flag (since the action runs in the asyncio loop
        # thread, but we want the test loop to observe completion via call_when_loop_starts).
        completed_event.set()
        result = MagicMock()
        result.name = "SUCCESS"
        return result

    with patch("agent_os.dispatcher.BT_POST_HOOK_ENABLED", True), patch.dict(
        "agent_os.bt.actions.ACTION_REGISTRY", \
            {"npc_sell_to_player": fake_npc_sell_to_player},
    ):
        events = []
        async for ev in dispatcher.say_stream(
            npc_id="npc_wang_boss_001",
            player_input="来份阳春面",
            npc_context=[],
            session_id="sess-test",
            trace_id="tr-test",
            player_id="demo-uuid",
            enable_bt_post_hook=True,
            pending_purchase={"product_id": 7, "currency": "gold"},
        ):
            events.append(ev)

        # Stream itself completed (last event is the done event)
        assert events[-1].complete is True

        # The fire-and-forget task should have run synchronously inside
        # the event loop (no await on upstream network). Yield once so
        # the scheduled task gets a chance to run before we assert.
        await asyncio.sleep(0)
        assert completed_event.is_set()
        assert captured_kwargs == {
            "product_id": 7,
            "currency": "gold",
            "player_id": "demo-uuid",
        }


@pytest.mark.asyncio
async def test_say_stream_without_post_hook_does_not_dispatch():
    """Default (no opt-in) → say_stream completes but does not schedule hook."""
    llm = MagicMock()
    pub = MagicMock()
    pub.publish_beat = AsyncMock()
    pub.publish_done = AsyncMock()

    dispatcher = ActionDispatcher(llm_client=llm, publisher=pub)

    async def fake_stream(req):
        yield {"text": "<end>", "finish_reason": "stop"}

    dispatcher.llm_client.stream = fake_stream

    dispatched = MagicMock()
    with patch("agent_os.dispatcher.BT_POST_HOOK_ENABLED", False), patch.dict(
        "agent_os.bt.actions.ACTION_REGISTRY", \
            {"npc_sell_to_player": dispatched},
    ):
        async for _ in dispatcher.say_stream(
            npc_id="npc_wang_boss_001",
            player_input="点菜",
            npc_context=[],
            session_id="sess-test",
            trace_id="tr-test",
            # Note: enable_bt_post_hook defaults to False AND env stays False
            pending_purchase={"product_id": 1, "currency": "gold"},
        ):
            pass

        # Give any (nonexistent) background task a moment
        await asyncio.sleep(0.05)
        assert dispatched.call_count == 0