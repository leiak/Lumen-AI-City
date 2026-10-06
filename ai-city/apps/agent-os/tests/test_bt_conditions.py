"""Condition registry tests (Phase C.1). 3 conditions: player_in_tile /
npc_state_equals / time_of_day_in."""
from __future__ import annotations

import pytest

from agent_os.bt.conditions import (
    CONDITION_REGISTRY,
    dispatch_condition,
    npc_state_equals,
    player_in_tile,
    time_of_day_in,
)
from agent_os.bt.state import BTState


def test_registry_has_3_conditions():
    expected = {"player_in_tile", "npc_state_equals", "time_of_day_in"}
    assert set(CONDITION_REGISTRY.keys()) == expected


# 1. player_in_tile
def test_condition_player_in_tile_match():
    state = BTState(player_position=(3, 4))
    assert player_in_tile(3, 4, state) is True


def test_condition_player_in_tile_mismatch():
    state = BTState(player_position=(3, 4))
    assert player_in_tile(0, 0, state) is False


def test_condition_player_in_tile_none_position():
    state = BTState()  # player_position=None
    assert player_in_tile(0, 0, state) is False


# 2. npc_state_equals
def test_condition_npc_state_equals_match():
    state = BTState(npc_state={"mood": "happy"})
    assert npc_state_equals("mood", "happy", state) is True


def test_condition_npc_state_equals_no_match():
    state = BTState(npc_state={"mood": "happy"})
    assert npc_state_equals("mood", "sad", state) is False


def test_condition_npc_state_equals_missing_key():
    state = BTState(npc_state={})
    assert npc_state_equals("mood", "happy", state) is False


# 3. time_of_day_in
def test_condition_time_of_day_in_match():
    state = BTState(time_of_day="morning")
    assert time_of_day_in(["morning", "noon"], state) is True


def test_condition_time_of_day_in_no_match():
    state = BTState(time_of_day="night")
    assert time_of_day_in(["morning", "noon"], state) is False


# dispatch
def test_dispatch_condition_routes_by_name():
    state = BTState(player_position=(1, 2))
    assert dispatch_condition("player_in_tile", [1, 2], state) is True
    assert dispatch_condition("player_in_tile", [9, 9], state) is False


def test_dispatch_condition_unknown_raises():
    state = BTState()
    with pytest.raises(ValueError):
        dispatch_condition("fake_condition", [], state)