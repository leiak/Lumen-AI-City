"""BT action registry (Phase C.1).

5 actions:
- ``move_to_tile(x, y, state)`` → SUCCESS, sets ``state.npc_move_target``
- ``say_to_player(text, emotion, state)`` → SUCCESS, appends to ``state.say_buffer``
- ``wait(seconds, state)`` → RUNNING, sets ``state.wait_until``
- ``set_npc_state(key, value, state)`` → SUCCESS, writes ``state.npc_state[key]``
- ``noop(state)`` → SUCCESS

Plus two synthetic test-only actions (``_fail`` / ``_running``) for evaluator
unit tests; they are not part of the public 5-action contract.
"""
from __future__ import annotations

import time
from typing import Any, Callable

from agent_os.bt.schema import Status
from agent_os.bt.state import BTState

# Maximum wait duration in seconds (1 hour). Prevents silent time-travel and
# overflow when BT configs supply negative or unbounded ``seconds`` values.
# Out-of-range inputs return FAILURE per BT convention so the parent
# composite (selector / sequence) can recover gracefully.
MAX_WAIT_SECONDS: float = 3600.0


def move_to_tile(x: int, y: int, state: BTState) -> Status:
    state.npc_move_target = (x, y)
    return Status.SUCCESS


def say_to_player(text: str, emotion: str, state: BTState) -> Status:
    state.say_buffer.append({"text": text, "emotion": emotion})
    return Status.SUCCESS


def wait(seconds: float, state: BTState) -> Status:
    if seconds < 0.0 or seconds > MAX_WAIT_SECONDS:
        return Status.FAILURE
    state.wait_until = time.time() + seconds
    return Status.RUNNING


def set_npc_state(key: str, value: Any, state: BTState) -> Status:
    state.npc_state[key] = value
    return Status.SUCCESS


def noop(state: BTState) -> Status:
    return Status.SUCCESS


# ---- W2.4: economy bridge -------------------------------------------------
# NPC BT action that calls economy-service POST /api/v1/wallet/purchase.
# Optional dep (httpx) imported lazily so agent-os remains usable without the
# economy stack (e.g. tests that don't exercise this action).

def npc_sell_to_player(
    product_id: int, currency: str, state: BTState,
) -> Status:
    """NPC 通过 product_id 售货给玩家。调 economy-service。

    失败（网络/余额不足/库存）返回 FAILURE（BT 短路）。
    """
    import httpx  # local import — optional dep
    import os

    base_url = os.environ.get(
        "ECONOMY_SERVICE_URL", "http://economy-service:8005",
    )
    trace_id = f"bt_tick_{state.tick_count}"

    try:
        with httpx.Client(timeout=5.0) as client:
            r = client.post(
                f"{base_url}/api/v1/wallet/purchase",
                json={
                    "user_id": state.player_id or "anonymous",
                    "product_id": product_id,
                    "currency": currency,
                    "idempotency_key": trace_id,
                    "trace_id": trace_id,
                },
            )
        if r.status_code == 200:
            data = r.json()
            state.last_purchase = data
            return Status.SUCCESS
        else:
            detail = r.json().get("detail", {})
            state.last_purchase_error = (
                f"{detail.get('code', 'R_018')}: {detail.get('msg', 'unknown')}"
            )
            return Status.FAILURE
    except (httpx.HTTPError, ValueError) as e:
        state.last_purchase_error = f"network: {e}"
        return Status.FAILURE


# ---- Test-only synthetic actions -----------------------------------------
# These are used by the evaluator tests to verify FAILURE/RUNNING propagation.
# They are NOT exported as part of the public action API.


def _fail(*args: Any, **kwargs: Any) -> Status:
    return Status.FAILURE


def _running(*args: Any, **kwargs: Any) -> Status:
    return Status.RUNNING


ACTION_REGISTRY: dict[str, Callable[..., Status]] = {
    "move_to_tile": move_to_tile,
    "say_to_player": say_to_player,
    "wait": wait,
    "set_npc_state": set_npc_state,
    "noop": noop,
    "npc_sell_to_player": npc_sell_to_player,  # W2.4
}

# Test-only synthetic actions used by evaluator unit tests to verify
# FAILURE/RUNNING propagation. NOT exported as part of the public action API
# — tests import ``TEST_ACTIONS`` directly.
TEST_ACTIONS: dict[str, Callable[..., Status]] = {
    "_fail": _fail,
    "_running": _running,
}


def dispatch_action(name: str, args: list[Any], state: BTState) -> Status:
    """Dispatch an action by name. Raises ``ValueError`` if unknown.

    Looks up the public ``ACTION_REGISTRY`` first, then falls back to
    ``TEST_ACTIONS`` so evaluator unit tests can drive failure/running flows
    without polluting the public 5-action contract.
    """
    fn = ACTION_REGISTRY.get(name) or TEST_ACTIONS.get(name)
    if fn is None:
        raise ValueError(f"unknown action: {name!r}")
    return fn(*args, state=state)


__all__ = [
    "ACTION_REGISTRY",
    "MAX_WAIT_SECONDS",
    "TEST_ACTIONS",
    "dispatch_action",
    "move_to_tile",
    "noop",
    "npc_sell_to_player",
    "say_to_player",
    "set_npc_state",
    "wait",
]