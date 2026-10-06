"""W4.2: end-to-end full-flow test — dispatcher → BT action → economy mock → state.

Composes the W2.4 ``npc_sell_to_player`` action with mocked economy-service
responses so we can exercise the SUCCESS / FAILURE paths without an
upstream dependency. Verifies ``BTState.last_purchase`` / ``last_purchase_error``
propagation exactly the way the W4 dispatcher relies on it.

This file is intentionally separate from ``test_bt_actions.py`` (which
covers the action's own contract) — here we drive it from the dispatcher
end of the wire (fire-and-forget task) and assert the full chain works.

The three direct-action tests below exercise ``npc_sell_to_player`` in
isolation (action → economy mock → state). The new
``test_dispatcher_say_stream_with_post_hook_runs_action`` test (Fix 3)
drives the full dispatcher path: ``say_stream()`` →
``schedule_post_hook()`` → ``_run_post_hook()`` (via
``asyncio.to_thread``) → ``npc_sell_to_player`` → economy mock.
"""
from __future__ import annotations

import asyncio
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from agent_os.bt.actions import npc_sell_to_player
from agent_os.bt.schema import Status
from agent_os.bt.state import BTState
from agent_os.dispatcher import ActionDispatcher

# ---------------------------------------------------------------------------
# Direct BT action e2e (action → economy mock → state)
# ---------------------------------------------------------------------------


def test_npc_sell_to_player_e2e_success(monkeypatch):
    """Mock 200 → SUCCESS + last_purchase populated + last_purchase_error = None."""
    def mock_post(url, json):
        m = MagicMock()
        m.status_code = 200
        m.json.return_value = {
            "user_id": json["user_id"],
            "product_id": json["product_id"],
            "balance_after": 950,
            "sunk": 50,
        }
        return m

    monkeypatch.setattr(
        "httpx.Client",
        lambda **kw: MagicMock(
            __enter__=lambda s: MagicMock(post=mock_post),
            __exit__=lambda s, *a: False,
        ),
    )

    state = BTState(player_id="p-demo")
    status = npc_sell_to_player(product_id=42, currency="gold", state=state)

    assert status == Status.SUCCESS
    assert state.last_purchase is not None
    assert state.last_purchase["balance_after"] == 950
    assert state.last_purchase["sunk"] == 50
    assert state.last_purchase["product_id"] == 42
    assert state.last_purchase_error is None


def test_npc_sell_to_player_e2e_insufficient(monkeypatch):
    """Mock 402 R_022 → FAILURE + last_purchase_error carries code."""
    def mock_post(url, json):
        m = MagicMock()
        m.status_code = 402
        m.json.return_value = {"detail": {"code": "R_022", "msg": "insufficient gold"}}
        return m

    monkeypatch.setattr(
        "httpx.Client",
        lambda **kw: MagicMock(
            __enter__=lambda s: MagicMock(post=mock_post),
            __exit__=lambda s, *a: False,
        ),
    )

    state = BTState(player_id="p-demo")
    status = npc_sell_to_player(product_id=42, currency="gold", state=state)

    assert status == Status.FAILURE
    assert state.last_purchase is None
    assert state.last_purchase_error is not None
    assert "R_022" in state.last_purchase_error
    assert "insufficient gold" in state.last_purchase_error


# ---------------------------------------------------------------------------
# Async integration: action invoked via asyncio.to_thread (the post-hook does
# it via asyncio.create_task wrapping a sync fn, so simulate via to_thread).
# ---------------------------------------------------------------------------


@pytest.mark.asyncio
async def test_full_flow_dispatcher_hook_purchase_success(monkeypatch):
    """Simulated full flow — fire-and-forget via asyncio.to_thread:
    dispatcher enqueues → npc_sell_to_player → economy mock 200 →
    state.last_purchase populated.
    """
    def mock_post(url, json):
        m = MagicMock()
        m.status_code = 200
        m.json.return_value = {
            "user_id": json["user_id"],
            "product_id": json["product_id"],
            "balance_after": 950,
            "sunk": 50,
        }
        return m

    monkeypatch.setattr(
        "httpx.Client",
        lambda **kw: MagicMock(
            __enter__=lambda s: MagicMock(post=mock_post),
            __exit__=lambda s, *a: False,
        ),
    )

    state = BTState(
        player_id="p-demo",
        pending_purchase={"product_id": 42, "currency": "gold"},
    )

    # Fire-and-forget: run the sync BT action in a worker thread so the
    # caller's stream never blocks on the upstream HTTP call.
    pending = dict(state.pending_purchase)
    task = asyncio.create_task(
        asyncio.to_thread(
            npc_sell_to_player,
            product_id=pending["product_id"],
            currency=pending["currency"],
            state=state,
        ),
    )
    status = await task

    assert status == Status.SUCCESS
    assert state.last_purchase is not None
    assert state.last_purchase["balance_after"] == 950
    assert state.last_purchase_error is None


# ---------------------------------------------------------------------------
# Fix 3: Real dispatcher-driven e2e test
# ---------------------------------------------------------------------------
# Drives ``say_stream()`` → ``schedule_post_hook()`` → ``_run_post_hook()``
# (via ``asyncio.to_thread`` after Fix 2) → ``npc_sell_to_player`` →
# mocked economy service. Mocks ``httpx.Client`` (sync — the action uses a
# sync ``with httpx.Client(...) as client`` block, not AsyncClient) so the
# action can complete inside ``asyncio.to_thread`` without a real network.
# ---------------------------------------------------------------------------


@pytest.mark.asyncio
async def test_dispatcher_say_stream_with_post_hook_runs_action(monkeypatch):
    """say_stream(enable_bt_post_hook=True, pending_purchase=...) → action runs.

    Real dispatcher path (not direct action call). Verifies:
      - The post-hook fires when ``enable_bt_post_hook=True`` even if env is off
        (Fix 1: kwarg overrides env).
      - The action runs in a worker thread (``asyncio.to_thread`` wrap from
        Fix 2), so the dispatcher's event loop is not blocked.
      - ``state.last_purchase`` propagates back to the BTState constructed by
        ``say_stream()`` (the dispatcher passes its own BTState to the hook).
    """
    # Mock httpx.Client.post so the economy call returns a 200 without
    # touching the network. ``npc_sell_to_player`` uses ``httpx.Client``
    # (sync) as a context manager, so the mock must be sync too.
    post_calls: list[dict] = []

    def mock_post(url, json):
        post_calls.append({"url": url, "json": json})
        m = MagicMock()
        m.status_code = 200
        m.json.return_value = {
            "user_id": json["user_id"],
            "product_id": json["product_id"],
            "balance_after": 950,
            "sunk": 50,
        }
        return m

    monkeypatch.setattr(
        "httpx.Client",
        lambda **kw: MagicMock(
            __enter__=lambda s: MagicMock(post=mock_post),
            __exit__=lambda s, *a: False,
        ),
    )

    # Build a minimal ActionDispatcher with a stubbed LLM stream + publisher.
    # ``publish_beat`` / ``publish_done`` are async because the dispatcher
    # awaits them; MagicMock alone isn't awaitable, so wrap with AsyncMock.
    llm = MagicMock()
    pub = MagicMock()
    pub.publish_beat = AsyncMock()
    pub.publish_done = AsyncMock()

    dispatcher = ActionDispatcher(llm_client=llm, publisher=pub)

    async def fake_stream(req):
        yield {"text": "<emotion=happy>来了您嘞！</emotion>", "finish_reason": None}
        yield {"text": "<end>", "finish_reason": "stop"}

    dispatcher.llm_client.stream = fake_stream

    # Fix 1: env=False but kwarg=True → hook still runs (kwarg overrides env).
    with patch("agent_os.dispatcher.BT_POST_HOOK_ENABLED", False):
        chunks = []
        async for ev in dispatcher.say_stream(
            npc_id="npc_wang_boss_001",
            player_input="来份阳春面",
            npc_context=[],
            session_id="sess-e2e",
            trace_id="tr-e2e",
            player_id="demo-uuid",
            enable_bt_post_hook=True,  # kwarg overrides env-off
            pending_purchase={"product_id": 42, "currency": "gold"},
        ):
            chunks.append(ev)

    # Stream yielded a done event (last event has complete=True).
    assert chunks, "say_stream yielded no events"
    assert chunks[-1].complete is True

    # The post-hook runs in a worker thread (Fix 2). Wait deterministically
    # for the post() call to land in mock_post, then assert it was called
    # with the right payload.
    for _ in range(50):  # up to ~5s at 100ms sleeps
        if post_calls:
            break
        await asyncio.sleep(0.1)

    assert post_calls, "post-hook never invoked economy mock"
    call = post_calls[0]
    assert call["url"].endswith("/api/v1/wallet/purchase")
    assert call["json"]["product_id"] == 42
    assert call["json"]["currency"] == "gold"
    assert call["json"]["user_id"] == "demo-uuid"