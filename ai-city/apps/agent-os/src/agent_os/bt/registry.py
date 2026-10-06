"""Standalone BTTree registry (separate from npc_registry for clean separation).

Used by the evaluator when resolving ``subtree`` node ``tree_id`` references.
"""
from __future__ import annotations

from agent_os.bt.errors import BTError
from agent_os.bt.schema import BTTree


class BTTreeRegistry:
    """In-memory registry of named BTTrees for ``subtree`` lookups."""

    def __init__(self) -> None:
        self._trees: dict[str, BTTree] = {}

    def register(self, tree_id: str, tree: BTTree) -> None:
        self._trees[tree_id] = tree

    def get(self, tree_id: str) -> BTTree:
        try:
            return self._trees[tree_id]
        except KeyError as e:
            raise BTError(f"subtree '{tree_id}' not registered") from e

    def has(self, tree_id: str) -> bool:
        return tree_id in self._trees


__all__ = ["BTTreeRegistry"]