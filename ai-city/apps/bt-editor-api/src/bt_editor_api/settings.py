"""Phase C.2 — environment-driven configuration for bt-editor-api.

Kept intentionally tiny: a module-level constants file so tests and
production share the same source of truth. Override via env vars in the
container / compose / k8s manifest.
"""
from __future__ import annotations

import os

# Postgres — matches docker-compose service ``postgres`` defaults.
DATABASE_URL: str = os.environ.get(
    "DATABASE_URL",
    "postgresql://aicity:aicity@localhost:5432/aicity",
)

# BT tree limits (Phase C.2 spec). 10 levels deep / 50 nodes is a sane
# ceiling for hand-authored trees while still leaving room for future
# expanded scenarios.
BT_TREE_DEPTH_LIMIT: int = int(os.environ.get("BT_TREE_DEPTH_LIMIT", "10"))
BT_TREE_NODE_LIMIT: int = int(os.environ.get("BT_TREE_NODE_LIMIT", "50"))


__all__ = [
    "BT_TREE_DEPTH_LIMIT",
    "BT_TREE_NODE_LIMIT",
    "DATABASE_URL",
]
