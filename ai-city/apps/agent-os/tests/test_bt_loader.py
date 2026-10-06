"""JSON → BT tree loader tests (Phase C.1)."""
from __future__ import annotations

import json

import pytest
from pydantic import ValidationError

from agent_os.bt.loader import load_tree, loads_tree
from agent_os.bt.schema import (
    ActionNode,
    BTTree,
    ConditionNode,
    DecoratorNode,
    SequenceNode,
)


def test_loader_parses_dict():
    raw = {"id": "root", "type": "sequence", "children": []}
    tree = load_tree(raw)
    assert isinstance(tree.root, SequenceNode)


def test_loader_handles_nested_composite():
    raw = {
        "id": "root",
        "type": "sequence",
        "children": [
            {
                "id": "sel1",
                "type": "selector",
                "children": [
                    {"id": "a", "type": "action", "name": "noop", "args": []},
                    {"id": "b", "type": "condition", "name": "player_in_tile", "args": [0, 0]},
                ],
            },
        ],
    }
    tree = load_tree(raw)
    assert isinstance(tree.root, SequenceNode)
    assert len(tree.root.children) == 1


def test_loader_recursive_nesting_3_levels():
    raw = {
        "id": "l0",
        "type": "sequence",
        "children": [
            {
                "id": "l1",
                "type": "selector",
                "children": [
                    {
                        "id": "l2",
                        "type": "decorator",
                        "kind": "inverter",
                        "child": {
                            "id": "l3",
                            "type": "condition",
                            "name": "player_in_tile",
                            "args": [1, 2],
                        },
                    },
                ],
            },
        ],
    }
    tree = load_tree(raw)
    # Walk down to deepest node
    seq = tree.root
    assert isinstance(seq, SequenceNode)
    sel = seq.children[0]
    assert sel.type.value == "selector"
    dec = sel.children[0]
    assert isinstance(dec, DecoratorNode)
    assert dec.kind == "inverter"
    assert dec.child is not None
    cond = dec.child
    assert isinstance(cond, ConditionNode)
    assert cond.name == "player_in_tile"
    assert cond.args == [1, 2]


def test_loader_loads_from_json_string():
    raw = json.dumps({"id": "x", "type": "action", "name": "noop", "args": []})
    tree = loads_tree(raw)
    assert isinstance(tree.root, ActionNode)


def test_loader_invalid_json_raises():
    with pytest.raises((ValidationError, ValueError, json.JSONDecodeError)):
        loads_tree("{not valid json")


def test_loader_missing_required_field_raises():
    """Loader must propagate Pydantic validation errors."""
    raw = {"id": "root"}  # no type
    with pytest.raises(ValidationError):
        load_tree(raw)


def test_loader_empty_children_ok():
    tree = load_tree({"id": "root", "type": "sequence", "children": []})
    assert tree.root.children == []


def test_loader_returns_bttree_rootmodel():
    tree = load_tree({"id": "x", "type": "action", "name": "noop", "args": []})
    assert isinstance(tree, BTTree)