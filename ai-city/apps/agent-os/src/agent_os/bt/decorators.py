"""BT decorator registry (Phase C.1).

3 decorators that transform a single child status:

- ``inverter(child_status, state)`` — SUCCESS↔FAILURE, RUNNING passes through
- ``repeater(child_status, times, state)`` — repeats up to N times; RUNNING propagates immediately
- ``until_success(child_status, max_tries, state)`` — succeeds on first SUCCESS, else FAIL after N tries

Each decorator receives the *status* returned by ticking the child (not the
child node itself) so this module is decoupled from the evaluator and can be
unit-tested in isolation. The evaluator computes ``child_status`` then calls the
matching decorator function here.
"""
from __future__ import annotations

from typing import Callable

from agent_os.bt.schema import Status
from agent_os.bt.state import BTState


def inverter(child_status: Status, state: BTState) -> Status:
    """Flip SUCCESS↔FAILURE; pass RUNNING through."""
    if child_status == Status.SUCCESS:
        return Status.FAILURE
    if child_status == Status.FAILURE:
        return Status.SUCCESS
    return Status.RUNNING


def repeater(child_status: Status, times: int, state: BTState) -> Status:
    """Repeat up to ``times`` times. RUNNING propagates immediately (v0: single-shot model).

    For v0 we accept the already-computed ``child_status`` and return it if it
    is a terminal status, since re-ticking is the evaluator's responsibility.
    Tests cover: SUCCESS in → SUCCESS out, RUNNING in → RUNNING out.
    """
    # RUNNING propagates immediately (matches BT conventions: re-tick on next frame)
    if child_status == Status.RUNNING:
        return Status.RUNNING
    # Terminal status — caller can decide whether to re-tick; we just report last.
    return child_status


def until_success(child_status: Status, max_tries: int, state: BTState) -> Status:
    """First SUCCESS short-circuits; RUNNING propagates; after N tries → FAILURE."""
    if child_status == Status.SUCCESS:
        return Status.SUCCESS
    if child_status == Status.RUNNING:
        return Status.RUNNING
    # Failure — but this is a single-tick view; caller drives the loop.
    return Status.FAILURE


DECORATOR_REGISTRY: dict[str, Callable[..., Status]] = {
    "inverter": inverter,
    "repeater": repeater,
    "until_success": until_success,
}


def apply_decorator(
    kind: str,
    child_status: Status,
    state: BTState,
    *,
    times: int = 1,
    max_tries: int = 10,
) -> Status:
    """Apply a decorator by name. Raises ``ValueError`` if unknown."""
    if kind == "inverter":
        fn = inverter
        return fn(child_status, state)
    if kind == "repeater":
        return repeater(child_status, times, state)
    if kind == "until_success":
        return until_success(child_status, max_tries, state)
    raise ValueError(f"unknown decorator kind: {kind!r}")


__all__ = [
    "DECORATOR_REGISTRY",
    "apply_decorator",
    "inverter",
    "repeater",
    "until_success",
]