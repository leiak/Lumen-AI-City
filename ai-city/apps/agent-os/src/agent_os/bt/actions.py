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


def move_to_tile(x: int, y: int, state: BTState) -> Status:
    state.npc_move_target = (x, y)
    return Status.SUCCESS


def say_to_player(text: str, emotion: str, state: BTState) -> Status:
    state.say_buffer.append({"text": text, "emotion": emotion})
    return Status.SUCCESS


def wait(seconds: float, state: BTState) -> Status:
    state.wait_until = time.time() + seconds
    return Status.RUNNING


def set_npc_state(key: str, value: Any, state: BTState) -> Status:
    state.npc_state[key] = value
    return Status.SUCCESS


def noop(state: BTState) -> Status:
    return Status.SUCCESS


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
    "TEST_ACTIONS",
    "dispatch_action",
    "move_to_tile",
    "noop",
    "say_to_player",
    "set_npc_state",
    "wait",
]