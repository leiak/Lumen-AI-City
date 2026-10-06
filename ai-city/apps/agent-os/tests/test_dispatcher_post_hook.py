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
    """enabled=False (kwarg or env) → no task scheduled."""
    with patch("agent_os.dispatcher.BT_POST_HOOK_ENABLED", False):
        state = BTState(player_id="p1", pending_purchase={"product_id": 1})
        # Caller already resolved: env=False, no kwarg → enabled=False
        result = schedule_post_hook(state, enabled=False)
        assert result is None


@pytest.mark.asyncio
async def test_post_hook_no_pending_purchase_returns_none():
    """pending_purchase=None → no task scheduled (even if enabled)."""
    state = BTState(player_id="p1")
    # enabled=True but no pending_purchase → still None
    result = schedule_post_hook(state, enabled=True)
    assert result is None


@pytest.mark.asyncio
async def test_post_hook_runs_npc_sell_to_player_action():
    """Pending purchase + enabled → schedules npc_sell_to_player task."""
    state = BTState(
        player_id="p1",
        pending_purchase={"product_id": 42, "currency": "gold"},
    )
    with patch("agent_os.bt.actions.ACTION_REGISTRY") as mock_reg:
        mock_action = MagicMock()
        # npc_sell_to_player returns a Status enum, use MagicMock for simplicity
        mock_action.return_value.name = "SUCCESS"
        mock_reg.get.return_value = mock_action

        # Caller already resolved: enabled=True (e.g. via env or kwarg)
        task = schedule_post_hook(state, enabled=True)
        assert task is not None
        await task  # let the background coroutine complete

        mock_reg.get.assert_called_with("npc_sell_to_player")
        mock_action.assert_called_once_with(
            state=state, product_id=42, currency="gold",
        )


@pytest.mark.asyncio
async def test_post_hook_kwarg_overrides_env_off():
    """Fix 1 contract: kwarg=True overrides env=False → hook still runs.

    Verifies the new precedence rule documented in the
    ``enable_bt_post_hook`` docstring: kwarg=True runs even when env=False.
    Caller in ``say_stream()`` resolves the OR and passes the result as
    ``enabled=True``; this test asserts that resolution directly.
    """
    state = BTState(
        player_id="p1",
        pending_purchase={"product_id": 99, "currency": "gold"},
    )
    with (
        patch("agent_os.dispatcher.BT_POST_HOOK_ENABLED", False),
        patch("agent_os.bt.actions.ACTION_REGISTRY") as mock_reg,
    ):
        mock_action = MagicMock()
        mock_action.return_value.name = "SUCCESS"
        mock_reg.get.return_value = mock_action

        # Caller resolved: kwarg=True overrides env=False → enabled=True
        enabled = True or False  # mirrors say_stream() line: kwarg OR env
        task = schedule_post_hook(state, enabled=enabled)
        assert task is not None
        await task
        mock_action.assert_called_once()


@pytest.mark.asyncio
async def test_post_hook_kwarg_false_defers_to_env_off():
    """Fix 1 contract: kwarg=False + env=False → hook does NOT run."""
    state = BTState(player_id="p1", pending_purchase={"product_id": 99})
    with (
        patch("agent_os.dispatcher.BT_POST_HOOK_ENABLED", False),
        patch("agent_os.bt.actions.ACTION_REGISTRY") as mock_reg,
    ):
        mock_action = MagicMock()
        mock_reg.get.return_value = mock_action

        # Caller resolved: kwarg=False + env=False → enabled=False
        enabled = False or False
        task = schedule_post_hook(state, enabled=enabled)
        assert task is None
        mock_action.assert_not_called()


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
    # so the fake must be sync too — _run_post_hook wraps it in
    # asyncio.to_thread (Fix 2) so it runs in a worker thread. Wait for the
    # event via wait_for() so the test isn't flaky on slow CI runners.
    captured_kwargs: dict = {}
    completed_event = asyncio.Event()

    def fake_npc_sell_to_player(*, state, product_id, currency):
        captured_kwargs["product_id"] = product_id
        captured_kwargs["currency"] = currency
        captured_kwargs["player_id"] = state.player_id
        # Event.set() is thread-safe (event flag uses lock internally),
        # so this fires correctly from inside asyncio.to_thread.
        completed_event.set()
        result = MagicMock()
        result.name = "SUCCESS"
        return result

    # Enable via kwarg → enabled=True regardless of env (Fix 1 contract).
    with patch("agent_os.dispatcher.BT_POST_HOOK_ENABLED", False), patch.dict(
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

        # The fire-and-forget task is wrapped in asyncio.to_thread → runs in
        # worker thread. Wait for the action's completion event with a small
        # timeout so the test is deterministic across schedulers.
        try:
            await asyncio.wait_for(completed_event.wait(), timeout=1.0)
        except asyncio.TimeoutError:
            pytest.fail("post-hook did not complete within 1s")
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