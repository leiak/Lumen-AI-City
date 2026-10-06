"""Pydantic schema tests for BT runtime (Phase C.1).

7 node types: sequence / selector / action / condition / decorator / subtree / llm.
"""
from __future__ import annotations

import pytest
from pydantic import ValidationError

from agent_os.bt.schema import (
    ActionNode,
    BTNode,
    BTTree,
    ConditionNode,
    DecoratorNode,
    LLMNode,
    NodeType,
    SelectorNode,
    SequenceNode,
    Status,
    SubTreeNode,
)


# ---- Enum sanity -----------------------------------------------------------


def test_node_type_has_7_values():
    assert len(NodeType) == 7


def test_status_has_3_values():
    assert {s.value for s in Status} == {"success", "failure", "running"}


# ---- Sequence / Selector composite nodes -----------------------------------


def test_schema_sequence_parses_json():
    tree = BTTree.model_validate({
        "id": "root",
        "type": "sequence",
        "children": [
            {"id": "c1", "type": "condition", "name": "player_in_tile", "args": [0, 1]},
        ],
    })
    assert isinstance(tree.root, SequenceNode)
    assert tree.root.id == "root"
    assert len(tree.root.children) == 1


def test_schema_selector_parses_json():
    tree = BTTree.model_validate({
        "id": "root",
        "type": "selector",
        "children": [],
    })
    assert isinstance(tree.root, SelectorNode)
    assert tree.root.type == NodeType.SELECTOR


# ---- Action / Condition leaf nodes -----------------------------------------


def test_schema_action_with_args_parses():
    tree = BTTree.model_validate({
        "id": "a1",
        "type": "action",
        "name": "move_to_tile",
        "args": [3, 4],
    })
    assert isinstance(tree.root, ActionNode)
    assert tree.root.name == "move_to_tile"
    assert tree.root.args == [3, 4]


def test_schema_condition_with_args_parses():
    tree = BTTree.model_validate({
        "id": "c1",
        "type": "condition",
        "name": "npc_state_equals",
        "args": ["mood", "happy"],
    })
    assert isinstance(tree.root, ConditionNode)
    assert tree.root.name == "npc_state_equals"


# ---- Decorator / SubTree / LLM --------------------------------------------


def test_schema_decorator_with_kind_and_child():
    tree = BTTree.model_validate({
        "id": "d1",
        "type": "decorator",
        "kind": "inverter",
        "child": {"id": "c1", "type": "condition", "name": "player_in_tile", "args": [0, 0]},
    })
    assert isinstance(tree.root, DecoratorNode)
    assert tree.root.kind == "inverter"
    assert tree.root.child is not None


def test_schema_subtree_with_tree_id():
    tree = BTTree.model_validate({
        "id": "s1",
        "type": "subtree",
        "tree_id": "patrol_route_a",
    })
    assert isinstance(tree.root, SubTreeNode)
    assert tree.root.tree_id == "patrol_route_a"


def test_schema_llm_with_choices_and_expected():
    tree = BTTree.model_validate({
        "id": "l1",
        "type": "llm",
        "prompt": "Choose: greet or ignore?",
        "choices": ["greet", "ignore"],
        "expected": "greet",
    })
    assert isinstance(tree.root, LLMNode)
    assert tree.root.expected == "greet"


# ---- Validation errors ----------------------------------------------------


def test_schema_missing_type_raises_validation_error():
    with pytest.raises(ValidationError):
        BTTree.model_validate({"id": "bad"})


def test_schema_unknown_type_raises_validation_error():
    with pytest.raises(ValidationError):
        BTTree.model_validate({"id": "x", "type": "totally-fake"})


def test_schema_unknown_node_type_raises_validation_error():
    """Nested children with bogus type should fail too."""
    with pytest.raises(ValidationError):
        BTTree.model_validate({
            "id": "root",
            "type": "sequence",
            "children": [{"id": "x", "type": "wat"}],
        })


# ---- Discriminated union dispatch ------------------------------------------


def test_schema_rootmodel_serializes_roundtrip():
    original = {
        "id": "root",
        "type": "sequence",
        "children": [{"id": "a", "type": "action", "name": "noop", "args": []}],
    }
    tree = BTTree.model_validate(original)
    dumped = tree.root.model_dump()
    assert dumped["id"] == "root"
    assert dumped["type"] == "sequence"


def test_schema_bt_node_alias_compatible():
    """BTNode alias must point to the root model."""
    assert BTNode is BTTree


# ---- DecoratorNode required child (Finding 4 fix) -------------------------


def test_decorator_without_child_raises_validation_error():
    """DecoratorNode.child must be present at parse-time, not tick-time."""
    # Build a decorator without ``child`` — Pydantic must reject this.
    with pytest.raises(ValidationError):
        DecoratorNode(id="d1", kind="inverter")

    # Same through BTTree.model_validate (loader path)
    with pytest.raises(ValidationError):
        BTTree.model_validate({"id": "d1", "type": "decorator", "kind": "inverter"})


def test_decorator_with_explicit_none_child_raises_validation_error():
    """Explicit child=null must also fail validation."""
    with pytest.raises(ValidationError):
        DecoratorNode(id="d1", kind="inverter", child=None)