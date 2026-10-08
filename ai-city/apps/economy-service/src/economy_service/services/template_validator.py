import json
from typing import Any

import yaml

MAX_DEPTH = 10
MAX_NODES = 50


class BTInvalidError(Exception):
    pass


class YamlInvalidError(Exception):
    pass


def validate_bt_skeleton(bt_json: str) -> None:
    try:
        root = json.loads(bt_json)
    except (json.JSONDecodeError, TypeError) as exc:
        raise BTInvalidError(f"BT not valid JSON: {exc}") from exc

    node_count = 0
    _walk(root, depth=0, node_count_ref=[node_count])


class _SagaSafeLoader(yaml.SafeLoader):
    pass


def _block_python_tag(loader: yaml.SafeLoader, tag_suffix: str, node: yaml.Node) -> None:
    raise YamlInvalidError(f"Forbidden YAML tag: tag:yaml.org,2002:python/{tag_suffix}")


_SagaSafeLoader.add_multi_constructor(
    "tag:yaml.org,2002:python/",
    _block_python_tag,
)


def validate_saga_yaml(yaml_text: str) -> None:
    try:
        yaml.load(yaml_text, Loader=_SagaSafeLoader)
    except yaml.YAMLError as exc:
        if isinstance(exc, yaml.MarkedYAMLError) and "Forbidden YAML tag" in str(exc):
            raise YamlInvalidError(str(exc)) from exc
        raise YamlInvalidError(f"YAML parse error: {exc}") from exc


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
