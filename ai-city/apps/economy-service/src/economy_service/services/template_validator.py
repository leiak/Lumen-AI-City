import json
from typing import Any

MAX_DEPTH = 10
MAX_NODES = 50


class BTInvalidError(Exception):
    pass


def validate_bt_skeleton(bt_json: str) -> None:
    try:
        root = json.loads(bt_json)
    except (json.JSONDecodeError, TypeError) as exc:
        raise BTInvalidError(f"BT not valid JSON: {exc}") from exc

    node_count = 0
    _walk(root, depth=0, node_count_ref=[node_count])


def _walk(node: Any, depth: int, node_count_ref: list[int]) -> None:
    if not isinstance(node, dict):
        raise BTInvalidError("BT node must be an object")

    node_count_ref[0] += 1
    if depth > MAX_DEPTH:
        raise BTInvalidError(f"BT exceeds max depth of {MAX_DEPTH}")
    if node_count_ref[0] > MAX_NODES:
        raise BTInvalidError(f"BT exceeds {MAX_NODES} nodes")

    children = node.get("children")
    if children is None:
        return
    if not isinstance(children, list):
        raise BTInvalidError("BT children must be a list")

    for child in children:
        _walk(child, depth + 1, node_count_ref)
