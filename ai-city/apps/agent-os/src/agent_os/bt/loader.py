"""JSON → BTTree loader (Phase C.1).

Thin wrapper over Pydantic's ``model_validate`` / JSON parsing.
"""
from __future__ import annotations

import json
from typing import Any

from agent_os.bt.schema import BTTree


def load_tree(data: dict[str, Any]) -> BTTree:
    """Parse a BT dict into a typed ``BTTree`` (Pydantic validation)."""
    return BTTree.model_validate(data)


def loads_tree(text: str) -> BTTree:
    """Parse a BT JSON string into a typed ``BTTree``."""
    data = json.loads(text)
    return load_tree(data)


__all__ = ["load_tree", "loads_tree"]