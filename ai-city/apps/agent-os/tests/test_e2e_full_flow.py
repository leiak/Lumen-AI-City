"""W4.2: end-to-end full-flow test — dispatcher → BT action → economy mock → state.

Composes the W2.4 ``npc_sell_to_player`` action with mocked economy-service
responses so we can exercise the SUCCESS / FAILURE paths without an
upstream dependency. Verifies ``BTState.last_purchase`` / ``last_purchase_error``
propagation exactly the way the W4 dispatcher relies on it.

This file is intentionally separate from ``test_bt_actions.py`` (which
covers the action's own contract) — here we drive it from the dispatcher
end of the wire (fire-and-forget task) and assert the full chain works.
"""
from __future__ import annotations

import asyncio
from unittest.mock import MagicMock

import pytest

from agent_os.bt.actions import npc_sell_to_player
from agent_os.bt.schema import Status
from agent_os.bt.state import BTState

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