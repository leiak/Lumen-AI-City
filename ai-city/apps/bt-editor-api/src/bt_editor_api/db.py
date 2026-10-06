"""Phase C.2 — asyncpg pool + BT tree validation helpers.

Two responsibilities live here:

1. **Pool management** — a module-level ``asyncpg.Pool`` singleton so the
   FastAPI endpoints can call ``await get_pool()`` without threading the
   pool through dependency injection. Tests inject a mock pool via
   ``set_pool()`` (see ``tests/conftest.py``).

2. **BT validation helpers** — ``depth`` / ``count_nodes`` walk the
   ``BTTree`` object graph produced by the C.1 loader. ``validate_tree``
   wires both helpers together with ``HTTPException`` mappings so the API
   endpoints can ``raise_for_status`` cleanly.
"""
from __future__ import annotations

from typing import Any

import asyncpg
from agent_os.bt.errors import BTError
from agent_os.bt.loader import load_tree
from agent_os.bt.schema import (
    DecoratorNode,
    SelectorNode,
    SequenceNode,
)
from agent_os.bt.state import BTState
from fastapi import HTTPException
from pydantic import ValidationError

from bt_editor_api.settings import DATABASE_URL

# ---- Pool singleton --------------------------------------------------------


_pool: asyncpg.Pool | None = None


async def get_pool() -> asyncpg.Pool:
    """Return the module-level asyncpg pool, creating it lazily on first call."""
    global _pool
    if _pool is None:
        _pool = await asyncpg.create_pool(
            DATABASE_URL,
            min_size=1,
            max_size=10,
        )
    return _pool


def set_pool(pool: asyncpg.Pool | None) -> None:
    """Test seam — install a mock pool (or ``None`` to reset)."""
    global _pool
    _pool = pool


# ---- Tree-shape helpers (pure-Python walks over agent_os.bt nodes) ---------


def _children_of(node: Any) -> list[Any]:
    """Return a node's recursive children list — uniform across composite nodes.

    * Sequence / Selector → ``node.children``
    * Decorator          → ``[node.child]`` if set, else ``[]``
    * Anything else      → ``[]`` (leaves, SubTreeNode routing hops)
    """
    if isinstance(node, (SequenceNode, SelectorNode)):
        return list(node.children)
    if isinstance(node, DecoratorNode):
        return [node.child] if node.child is not None else []
    return []


def depth(node: Any, current: int = 1) -> int:
    """Return the maximum depth of the tree rooted at ``node``.

    A leaf node has depth 1; a composite whose deepest child is N levels
    below has depth 1 + N. ``SubTreeNode`` is counted as depth 1 (we don't
    resolve the referenced tree — its shape is validated independently).
    """
    kids = _children_of(node)
    if not kids:
        return current
    return max(depth(kid, current + 1) for kid in kids)


def count_nodes(node: Any) -> int:
    """Return the total number of nodes in the tree rooted at ``node``.

    Leaves count as 1; composites count as 1 + sum(child counts).
    ``SubTreeNode`` contributes only 1 — the referenced tree is validated
    on its own when saved.
    """
    return 1 + sum(count_nodes(kid) for kid in _children_of(node))


# ---- Public validation entry point -----------------------------------------


def validate_tree(tree_json: dict, depth_limit: int, node_limit: int) -> Any:
    """Validate a BT JSON dict using the C.1 loader + depth/node limits.

    Returns the parsed ``BTTree`` on success (callers may want it for
    simulate; the save path discards it).

    Raises:
        HTTPException(400, R_019)  — loader rejected the JSON (bad shape,
            unknown discriminator, missing required field).
        HTTPException(422, R_019)  — tree exceeds depth_limit or node_limit.
    """
    try:
        tree = load_tree(tree_json)
    except (ValidationError, BTError, ValueError) as e:
        raise HTTPException(
            status_code=400,
            detail={"code": "R_019", "msg": f"BT_PARSE_FAIL: {e}"},
        ) from e

    actual_depth = depth(tree.root)
    if actual_depth > depth_limit:
        raise HTTPException(
            status_code=422,
            detail={
                "code": "R_019",
                "msg": f"BT_PARSE_FAIL: depth {actual_depth} > limit {depth_limit}",
            },
        )

    actual_count = count_nodes(tree.root)
    if actual_count > node_limit:
        raise HTTPException(
            status_code=422,
            detail={
                "code": "R_019",
                "msg": f"BT_PARSE_FAIL: node count {actual_count} > limit {node_limit}",
            },
        )

    return tree


def build_bt_state(state_payload: dict) -> BTState:
    """Construct a ``BTState`` from a simulate request's ``state`` dict."""
    pos = state_payload.get("player_position")
    player_pos: tuple[int, int] | None = None
    if isinstance(pos, (list, tuple)) and len(pos) == 2:
        player_pos = (int(pos[0]), int(pos[1]))
    return BTState(
        player_position=player_pos,
        npc_state=dict(state_payload.get("npc_state") or {}),
        time_of_day=str(state_payload.get("time_of_day", "noon")),
        max_ticks=int(state_payload.get("max_ticks", 100)),
    )


def state_to_dict(state: BTState) -> dict:
    """Serialise a ``BTState`` to a JSON-safe dict for the response body."""
    return {
        "tick_count": state.tick_count,
        "player_position": list(state.player_position) if state.player_position else None,
        "npc_state": dict(state.npc_state),
        "npc_move_target": list(state.npc_move_target) if state.npc_move_target else None,
        "say_buffer": list(state.say_buffer),
        "wait_until": state.wait_until,
        "time_of_day": state.time_of_day,
    }


__all__ = [
    "build_bt_state",
    "count_nodes",
    "depth",
    "get_pool",
    "set_pool",
    "state_to_dict",
    "validate_tree",
]
