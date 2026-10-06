"""Action registry tests (Phase C.1). 5 actions: move_to_tile / say_to_player /
wait / set_npc_state / noop."""
from __future__ import annotations

import time

from agent_os.bt.actions import (
    ACTION_REGISTRY,
    dispatch_action,
    move_to_tile,
    noop,
    say_to_player,
    set_npc_state,
    wait,
)
from agent_os.bt.schema import Status
from agent_os.bt.state import BTState


def test_registry_has_5_actions():
    expected = {"move_to_tile", "say_to_player", "wait", "set_npc_state", "noop"}
    assert set(ACTION_REGISTRY.keys()) == expected


# 1. move_to_tile
def test_action_move_to_tile_sets_target():
    state = BTState()
    status = move_to_tile(2, 3, state)
    assert status == Status.SUCCESS
    assert state.npc_move_target == (2, 3)


# 2. say_to_player
def test_action_say_to_player_appends():
    state = BTState()
    status = say_to_player("hello there", "happy", state)
    assert status == Status.SUCCESS
    assert state.say_buffer == [{"text": "hello there", "emotion": "happy"}]
    say_to_player("another", "sad", state)
    assert len(state.say_buffer) == 2
    assert state.say_buffer[1]["emotion"] == "sad"


# 3. wait
def test_action_wait_returns_running():
    state = BTState()
    status = wait(2.5, state)
    assert status == Status.RUNNING
    assert state.wait_until is not None
    assert state.wait_until >= time.time() + 2.0  # roughly 2.5s in future


# 3a. wait — bounds checking (Finding 3 fix)
def test_action_wait_zero_returns_running():
    """wait(0.0) is on the boundary and must remain RUNNING with wait_until set."""
    state = BTState()
    status = wait(0.0, state)
    assert status == Status.RUNNING
    assert state.wait_until is not None


def test_action_wait_negative_seconds_returns_failure():
    """Negative seconds would silently rewind wait_until; reject as FAILURE."""
    state = BTState()
    status = wait(-1.0, state)
    assert status == Status.FAILURE
    # wait_until must NOT be touched on failure (no partial side-effects).
    assert state.wait_until is None


def test_action_wait_huge_seconds_returns_failure():
    """Seconds > MAX_WAIT_SECONDS (1h) overflows / silently hangs; reject."""
    from agent_os.bt.actions import MAX_WAIT_SECONDS

    state = BTState()
    # Just over the cap
    status = wait(MAX_WAIT_SECONDS + 1.0, state)
    assert status == Status.FAILURE
    assert state.wait_until is None


def test_action_wait_max_boundary_returns_running():
    """Exactly at MAX_WAIT_SECONDS is allowed."""
    from agent_os.bt.actions import MAX_WAIT_SECONDS

    state = BTState()
    status = wait(MAX_WAIT_SECONDS, state)
    assert status == Status.RUNNING
    assert state.wait_until is not None


# 4. set_npc_state
def test_action_set_npc_state():
    state = BTState()
    status = set_npc_state("mood", "happy", state)
    assert status == Status.SUCCESS
    assert state.npc_state["mood"] == "happy"


# 5. noop
def test_action_noop():
    state = BTState()
    status = noop(state)
    assert status == Status.SUCCESS


# dispatch_action
def test_dispatch_action_routes_by_name():
    state = BTState()
    status = dispatch_action("move_to_tile", [1, 1], state)
    assert status == Status.SUCCESS
    assert state.npc_move_target == (1, 1)


def test_dispatch_action_unknown_raises():
    state = BTState()
    import pytest
    with pytest.raises(ValueError):
        dispatch_action("totally_fake_action", [], state)