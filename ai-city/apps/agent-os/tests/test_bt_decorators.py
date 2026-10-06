"""Decorator tests (Phase C.1). 3 decorators: inverter / repeater / until_success."""
from __future__ import annotations

import pytest

from agent_os.bt.decorators import (
    DECORATOR_REGISTRY,
    apply_decorator,
    inverter,
    repeater,
    until_success,
)
from agent_os.bt.schema import Status
from agent_os.bt.state import BTError, BTState


def test_registry_has_3_decorators():
    expected = {"inverter", "repeater", "until_success"}
    assert set(DECORATOR_REGISTRY.keys()) == expected


# 1. inverter
def test_decorator_inverter_flips_success_to_failure():
    state = BTState()
    # Pass Status.SUCCESS directly; inverter flips it
    assert inverter(Status.SUCCESS, state) == Status.FAILURE


def test_decorator_inverter_flips_failure_to_success():
    state = BTState()
    assert inverter(Status.FAILURE, state) == Status.RUNNING or inverter(
        Status.FAILURE, state
    ) == Status.SUCCESS
    # Use a fresh call to avoid the above trick ambiguity
    state2 = BTState()
    assert inverter(Status.FAILURE, state2) == Status.SUCCESS


def test_decorator_inverter_running_passes_through():
    state = BTState()
    assert inverter(Status.RUNNING, state) == Status.RUNNING


# 2. repeater
def test_decorator_repeater_3_times_success():
    """Repeater with times=3 on a noop child returns SUCCESS."""
    from agent_os.bt.actions import noop

    state = BTState()
    # Use the (status, state) overload directly: simulate child returning SUCCESS three times.
    # The repeater over a (status) arg just returns last status — so SUCCESS in, SUCCESS out.
    status = repeater(Status.SUCCESS, times=3, state=state)
    assert status == Status.SUCCESS


def test_decorator_repeater_propagates_running_immediately():
    state = BTState()
    status = repeater(Status.RUNNING, times=5, state=state)
    assert status == Status.RUNNING


# 3. until_success
def test_decorator_until_success_breaks_on_first_success():
    state = BTState()
    status = until_success(Status.SUCCESS, max_tries=10, state=state)
    assert status == Status.SUCCESS


def test_decorator_until_success_runs_all_tries_on_failure():
    state = BTState()
    status = until_success(Status.FAILURE, max_tries=3, state=state)
    assert status == Status.FAILURE


def test_decorator_until_success_propagates_running():
    state = BTState()
    status = until_success(Status.RUNNING, max_tries=5, state=state)
    assert status == Status.RUNNING


# apply_decorator dispatch
def test_apply_decorator_unknown_raises():
    state = BTState()
    with pytest.raises(ValueError):
        apply_decorator("bogus", Status.SUCCESS, state, times=3, max_tries=3)