"""Phase C.2 — db.py helpers (depth, count, validation) unit tests.

These tests exercise the pure-Python validation helpers without touching
asyncpg — they exist to lock in the limits (depth ≤ BT_TREE_DEPTH_LIMIT,
nodes ≤ BT_TREE_NODE_LIMIT) so a regression in the limits is caught at
unit-test time, not via an HTTP roundtrip.
"""
from __future__ import annotations

import pytest
from agent_os.bt.loader import load_tree
from bt_editor_api import db as db_mod

# ---- depth helper ---------------------------------------------------------


def test_depth_single_node_is_one():
    tree = load_tree({"id": "root", "type": "action", "name": "noop", "args": []})
    assert db_mod.depth(tree.root) == 1


def test_depth_nested_sequence_grows():
    # sequence (1) → action (2) inside
    tree = load_tree({"id": "root", "type": "sequence", "children": [
        {"id": "a", "type": "action", "name": "noop", "args": []},
    ]})
    assert db_mod.depth(tree.root) == 2


def test_depth_decorator_counts_inner_child():
    # decorator(1) → sequence(2) → action(3)
    tree = load_tree({"id": "root", "type": "decorator", "kind": "inverter", "child": {
        "id": "s", "type": "sequence", "children": [
            {"id": "a", "type": "action", "name": "noop", "args": []},
        ],
    }})
    assert db_mod.depth(tree.root) == 3


def test_depth_subtree_counts_as_leaf():
    """SubTreeNode is a routing hop — counted as depth 1 (we don't resolve)."""
    tree = load_tree({"id": "root", "type": "subtree", "tree_id": "inner"})
    assert db_mod.depth(tree.root) == 1


# ---- count helper ---------------------------------------------------------


def test_count_single_node():
    tree = load_tree({"id": "root", "type": "action", "name": "noop", "args": []})
    assert db_mod.count_nodes(tree.root) == 1


def test_count_walks_full_tree():
    tree = load_tree({"id": "root", "type": "sequence", "children": [
        {"id": "a", "type": "action", "name": "noop", "args": []},
        {"id": "b", "type": "selector", "children": [
            {"id": "c", "type": "action", "name": "noop", "args": []},
            {"id": "d", "type": "action", "name": "noop", "args": []},
        ]},
    ]})
    # root(1) + a(1) + b(1) + c(1) + d(1) = 5
    assert db_mod.count_nodes(tree.root) == 5


def test_count_subtree_counts_only_one():
    """SubTreeNode is a routing hop — don't recurse into referenced tree."""
    tree = load_tree({"id": "root", "type": "sequence", "children": [
        {"id": "s", "type": "subtree", "tree_id": "inner"},
    ]})
    assert db_mod.count_nodes(tree.root) == 2  # root + subtree


# ---- validate_tree (limit enforcement) ------------------------------------


def test_validate_tree_within_limits_passes():
    """A small valid tree should not raise."""
    tree_json = {"id": "root", "type": "sequence", "children": [
        {"id": "a", "type": "action", "name": "noop", "args": []},
    ]}
    db_mod.validate_tree(tree_json, depth_limit=10, node_limit=50)  # no raise


def test_validate_tree_depth_exceeded_raises_422():
    """Tree deeper than depth_limit raises HTTPException(422)."""
    from fastapi import HTTPException

    def _branch(depth: int) -> dict:
        if depth == 0:
            return {"id": f"a{depth}", "type": "action", "name": "noop", "args": []}
        return {"id": f"a{depth}", "type": "sequence", "children": [_branch(depth - 1)]}

    deep = _branch(15)
    with pytest.raises(HTTPException) as exc_info:
        db_mod.validate_tree(deep, depth_limit=10, node_limit=50)
    assert exc_info.value.status_code == 422
    assert exc_info.value.detail["code"] == "R_019"


def test_validate_tree_node_count_exceeded_raises_422():
    from fastapi import HTTPException

    children = [
        {"id": f"a{i}", "type": "action", "name": "noop", "args": []}
        for i in range(51)
    ]
    tree_json = {"id": "root", "type": "sequence", "children": children}
    with pytest.raises(HTTPException) as exc_info:
        db_mod.validate_tree(tree_json, depth_limit=10, node_limit=50)
    assert exc_info.value.status_code == 422
    assert "node" in exc_info.value.detail["msg"].lower()


def test_validate_tree_invalid_json_raises_400():
    from fastapi import HTTPException

    with pytest.raises(HTTPException) as exc_info:
        db_mod.validate_tree({"id": "x"}, depth_limit=10, node_limit=50)
    assert exc_info.value.status_code == 400
    assert exc_info.value.detail["code"] == "R_019"
