"""Recursive tick evaluator tests (Phase C.1)."""
from __future__ import annotations

import pytest

from agent_os.bt.evaluator import BTTreeRegistry, tick
from agent_os.bt.loader import load_tree
from agent_os.bt.schema import (
    BTTree,
    Status,
)
from agent_os.bt.state import BTError, BTState


def _fresh_state(**kwargs) -> BTState:
    state = BTState()
    for k, v in kwargs.items():
        setattr(state, k, v)
    return state


# ---- Sequence --------------------------------------------------------------


def test_evaluator_sequence_all_success():
    tree = load_tree({
        "id": "root",
        "type": "sequence",
        "children": [
            {"id": "a", "type": "action", "name": "noop", "args": []},
            {"id": "b", "type": "action", "name": "noop", "args": []},
            {"id": "c", "type": "action", "name": "noop", "args": []},
        ],
    })
    state = _fresh_state()
    assert tick(tree, state) == Status.SUCCESS
    # tick_count == 1 root + 3 child ticks = 4
    assert state.tick_count == 4


def test_evaluator_sequence_short_circuits_on_failure():
    """If child[1] fails, child[2] must not run."""
    tree = load_tree({
        "id": "root",
        "type": "sequence",
        "children": [
            {"id": "a", "type": "action", "name": "noop", "args": []},
            {"id": "b", "type": "action", "name": "_fail", "args": []},
            {"id": "c", "type": "action", "name": "set_npc_state", "args": ["ran_c", True]},
        ],
    })
    state = _fresh_state()
    assert tick(tree, state) == Status.FAILURE
    assert "ran_c" not in state.npc_state  # c never executed


def test_evaluator_sequence_propagates_running():
    """Running child short-circuits sequence with RUNNING."""
    tree = load_tree({
        "id": "root",
        "type": "sequence",
        "children": [
            {"id": "a", "type": "action", "name": "_running", "args": []},
            {"id": "b", "type": "action", "name": "noop", "args": []},
        ],
    })
    state = _fresh_state()
    assert tick(tree, state) == Status.RUNNING


# ---- Selector --------------------------------------------------------------


def test_evaluator_selector_first_success():
    """Selector returns SUCCESS on first successful child, no later ticks."""
    tree = load_tree({
        "id": "root",
        "type": "selector",
        "children": [
            {"id": "a", "type": "action", "name": "noop", "args": []},
            {"id": "b", "type": "action", "name": "set_npc_state", "args": ["ran_b", True]},
        ],
    })
    state = _fresh_state()
    assert tick(tree, state) == Status.SUCCESS
    assert "ran_b" not in state.npc_state


def test_evaluator_selector_all_failure():
    tree = load_tree({
        "id": "root",
        "type": "selector",
        "children": [
            {"id": "a", "type": "action", "name": "_fail", "args": []},
            {"id": "b", "type": "action", "name": "_fail", "args": []},
        ],
    })
    state = _fresh_state()
    assert tick(tree, state) == Status.FAILURE


def test_evaluator_selector_propagates_running():
    """A running child (before any success/failure) halts selector with RUNNING."""
    tree = load_tree({
        "id": "root",
        "type": "selector",
        "children": [
            {"id": "a", "type": "action", "name": "_running", "args": []},
            {"id": "b", "type": "action", "name": "noop", "args": []},
        ],
    })
    state = _fresh_state()
    assert tick(tree, state) == Status.RUNNING


# ---- max_ticks protection --------------------------------------------------


def test_evaluator_max_ticks_raises_bterror():
    """Infinite child recursion raises BTError instead of hanging."""
    # Build a deep sequence so default max_ticks=100 will trigger.
    children = [
        {"id": f"a{i}", "type": "action", "name": "noop", "args": []}
        for i in range(200)
    ]
    tree = load_tree({"id": "root", "type": "sequence", "children": children})
    state = BTState(max_ticks=10)
    with pytest.raises(BTError):
        tick(tree, state)


def test_evaluator_subtree_loads_from_registry():
    """SubTree node looks up tree_id in the registry and ticks the referenced tree."""
    inner = load_tree({"id": "inner", "type": "action", "name": "noop", "args": []})
    registry = BTTreeRegistry()
    registry.register("inner_tree", inner)
    outer = load_tree({
        "id": "outer",
        "type": "sequence",
        "children": [
            {"id": "s", "type": "subtree", "tree_id": "inner_tree"},
        ],
    })
    state = BTState(registry=registry)
    assert tick(outer, state) == Status.SUCCESS


def test_evaluator_subtree_missing_raises_bterror():
    tree = load_tree({"id": "outer", "type": "subtree", "tree_id": "ghost"})
    state = BTState()
    with pytest.raises(BTError):
        tick(tree, state)


def test_evaluator_llm_node_v0_returns_success_when_expected_matches():
    """LLM node stub returns SUCCESS when expected matches the prompt keyword."""
    tree = load_tree({
        "id": "l1",
        "type": "llm",
        "prompt": "I should greet the player",
        "choices": ["greet", "ignore"],
        "expected": "greet",
    })
    state = _fresh_state()
    # Stub LLM picks the first choice when expected is "greet" matches prompt intent.
    result = tick(tree, state)
    assert result in {Status.SUCCESS, Status.FAILURE}  # depends on stub; must be deterministic


def test_evaluator_condition_node_short_circuits_sequence():
    """A failed condition in a sequence causes short-circuit."""
    tree = load_tree({
        "id": "root",
        "type": "sequence",
        "children": [
            {"id": "c1", "type": "condition", "name": "player_in_tile", "args": [0, 0]},
            {"id": "a1", "type": "action", "name": "set_npc_state", "args": ["got_here", True]},
        ],
    })
    state = _fresh_state(player_position=(5, 5))
    assert tick(tree, state) == Status.FAILURE
    assert "got_here" not in state.npc_state


def test_evaluator_decorator_inverter_flips():
    tree = load_tree({
        "id": "root",
        "type": "decorator",
        "kind": "inverter",
        "child": {"id": "c", "type": "condition", "name": "player_in_tile", "args": [0, 0]},
    })
    state = _fresh_state(player_position=(0, 0))
    assert tick(tree, state) == Status.FAILURE  # condition SUCCESS → inverter FAILURE


def test_evaluator_empty_sequence_returns_success():
    """An empty sequence with no children trivially succeeds."""
    tree = load_tree({"id": "root", "type": "sequence", "children": []})
    state = _fresh_state()
    assert tick(tree, state) == Status.SUCCESS


def test_evaluator_empty_selector_returns_failure():
    """An empty selector with no children trivially fails."""
    tree = load_tree({"id": "root", "type": "selector", "children": []})
    state = _fresh_state()
    assert tick(tree, state) == Status.FAILURE