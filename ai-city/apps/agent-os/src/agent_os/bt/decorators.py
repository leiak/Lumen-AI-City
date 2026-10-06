"""BT decorator registry (Phase C.1 + C.1 fixes).

3 decorators that transform a single child status:

- ``inverter(child_status, state)`` — SUCCESS↔FAILURE, RUNNING passes through
- ``repeater(child_status, times, state)`` — repeats up to N times; RUNNING propagates immediately
- ``until_success(child_status, max_tries, state)`` — succeeds on first SUCCESS, else FAIL after N tries

Each decorator receives the *status* returned by ticking the child (not the
child node itself) so this module is decoupled from the evaluator and can be
unit-tested in isolation.

Multi-tick behaviour is driven by ``tick_decorator(node, state, tick_child_fn)``
which loops N times and feeds each iteration's status back into the matching
pure decorator. Single-tick callers keep using ``apply_decorator``.
"""
from __future__ import annotations

from typing import Callable

from agent_os.bt.schema import DecoratorNode, Status
from agent_os.bt.state import BTState


def inverter(child_status: Status, state: BTState) -> Status:
    """Flip SUCCESS↔FAILURE; pass RUNNING through."""
    if child_status == Status.SUCCESS:
        return Status.FAILURE
    if child_status == Status.FAILURE:
        return Status.SUCCESS
    return Status.RUNNING


def repeater(child_status: Status, times: int, state: BTState) -> Status:
    """Single-tick status transform.

    Returns the child status as-is when it is terminal; RUNNING propagates
    immediately. The multi-tick loop is driven by ``tick_decorator`` — this
    function is the pure per-iteration reducer.
    """
    # RUNNING propagates immediately (matches BT conventions: re-tick on next frame)
    if child_status == Status.RUNNING:
        return Status.RUNNING
    # Terminal status — caller (tick_decorator) decides whether to re-tick.
    return child_status


def until_success(child_status: Status, max_tries: int, state: BTState) -> Status:
    """Single-tick status transform.

    SUCCESS short-circuits the multi-tick loop, RUNNING propagates, FAILURE
    means "try again" up to ``max_tries``. Pure reducer for ``tick_decorator``.
    """
    if child_status == Status.SUCCESS:
        return Status.SUCCESS
    if child_status == Status.RUNNING:
        return Status.RUNNING
    # Failure — caller (tick_decorator) drives the retry loop.
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
        return inverter(child_status, state)
    if kind == "repeater":
        return repeater(child_status, times, state)
    if kind == "until_success":
        return until_success(child_status, max_tries, state)
    raise ValueError(f"unknown decorator kind: {kind!r}")


def tick_decorator(
    node: DecoratorNode,
    state: BTState,
    tick_child_fn: Callable[[object, BTState], Status],
) -> Status:
    """Drive a decorator node's multi-tick semantics.

    For ``inverter`` this is single-tick (no count). For ``repeater(times=N)``
    it ticks the child up to N times, returning the last terminal status or
    RUNNING on first propagation. For ``until_success(max_tries=N)`` it ticks
    the child up to N times, returning on first SUCCESS, RUNNING propagates,
    and FAILURE-after-N-tries yields FAILURE.

    ``tick_child_fn`` is injected so this module stays decoupled from the
    evaluator (and its max_ticks accounting).
    """
    if node.child is None:
        # Defensive: schema validation should already have rejected this, but
        # if a DecoratorNode was constructed bypassing the validator we still
        # raise a clear error rather than crash on ``None``.
        raise ValueError(f"DecoratorNode {node.id!r} is missing its child")

    if node.kind == "inverter":
        child_status = tick_child_fn(node.child, state)
        return inverter(child_status, state)

    if node.kind == "repeater":
        last_status: Status = Status.SUCCESS  # default if times <= 0
        for _ in range(node.times):
            child_status = tick_child_fn(node.child, state)
            last_status = repeater(child_status, node.times, state)
            if last_status == Status.RUNNING:
                return Status.RUNNING
        return last_status

    if node.kind == "until_success":
        last_status: Status = Status.FAILURE  # default if max_tries <= 0
        for _ in range(node.max_tries):
            child_status = tick_child_fn(node.child, state)
            last_status = until_success(child_status, node.max_tries, state)
            if last_status == Status.SUCCESS:
                return Status.SUCCESS
            if last_status == Status.RUNNING:
                return Status.RUNNING
        return last_status

    raise ValueError(f"unknown decorator kind: {node.kind!r}")


__all__ = [
    "DECORATOR_REGISTRY",
    "apply_decorator",
    "inverter",
    "repeater",
    "tick_decorator",
    "until_success",
]
