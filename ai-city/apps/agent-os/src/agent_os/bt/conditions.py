"""BT condition registry (Phase C.1).

3 conditions:
- ``player_in_tile(x, y, state)`` → bool
- ``npc_state_equals(key, value, state)`` → bool
- ``time_of_day_in(allowed, state)`` → bool
"""
from __future__ import annotations

from typing import Any, Sequence

from agent_os.bt.state import BTState


def player_in_tile(x: int, y: int, state: BTState) -> bool:
    return state.player_position == (x, y)


def npc_state_equals(key: str, value: Any, state: BTState) -> bool:
    return state.npc_state.get(key) == value


def time_of_day_in(allowed: Sequence[str], state: BTState) -> bool:
    return state.time_of_day in list(allowed)


CONDITION_REGISTRY: dict[str, Any] = {
    "player_in_tile": player_in_tile,
    "npc_state_equals": npc_state_equals,
    "time_of_day_in": time_of_day_in,
}


def dispatch_condition(name: str, args: list[Any], state: BTState) -> bool:
    """Dispatch a condition by name. Raises ``ValueError`` if unknown."""
    try:
        fn = CONDITION_REGISTRY[name]
    except KeyError as e:
        raise ValueError(f"unknown condition: {name!r}") from e
    return fn(*args, state=state)


__all__ = [
    "CONDITION_REGISTRY",
    "dispatch_condition",
    "npc_state_equals",
    "player_in_tile",
    "time_of_day_in",
]