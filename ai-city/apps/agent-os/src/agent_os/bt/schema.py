"""Pydantic schema for BT runtime (Phase C.1).

7 node types modeled as a Pydantic v2 discriminated union keyed on ``type``:
- ``sequence`` — composite; all children SUCCESS → SUCCESS, else propagate FAIL/RUN
- ``selector`` — composite; first SUCCESS or RUNNING wins, all FAIL → FAIL
- ``action``   — leaf; dispatches a named action against BTState
- ``condition``— leaf; evaluates a named condition against BTState
- ``decorator``— wraps a single child; transforms its status (inverter/repeater/until_success)
- ``subtree``  — references another BT by ``tree_id`` via ``BTTreeRegistry``
- ``llm``      — calls LLM (v0: deterministic stub); returns SUCCESS/FAIL based on stub

``BTTree`` is a thin ``RootModel``-like container whose ``model_validate``
dispatches to the right node class via the ``type`` discriminator field. The
parsed root node is reachable through ``.root``. ``BTNode`` is an alias for
``BTTree`` for spec compatibility.
"""
from __future__ import annotations

from enum import Enum
from typing import Annotated, Any, Literal, Union

from pydantic import BaseModel, ConfigDict, Field, RootModel, ValidationError


class NodeType(str, Enum):
    SEQUENCE = "sequence"
    SELECTOR = "selector"
    ACTION = "action"
    CONDITION = "condition"
    DECORATOR = "decorator"
    SUBTREE = "subtree"
    LLM = "llm"


class Status(str, Enum):
    SUCCESS = "success"
    FAILURE = "failure"
    RUNNING = "running"


class BTNodeBase(BaseModel):
    """Base for all BT nodes. ``id`` is mandatory; ``type`` is discriminator."""

    model_config = ConfigDict(extra="forbid")
    id: str


class SequenceNode(BTNodeBase):
    type: Literal[NodeType.SEQUENCE] = NodeType.SEQUENCE
    children: list["BTNodeType"] = Field(default_factory=list)


class SelectorNode(BTNodeBase):
    type: Literal[NodeType.SELECTOR] = NodeType.SELECTOR
    children: list["BTNodeType"] = Field(default_factory=list)


class ActionNode(BTNodeBase):
    type: Literal[NodeType.ACTION] = NodeType.ACTION
    name: str
    args: list[Any] = Field(default_factory=list)


class ConditionNode(BTNodeBase):
    type: Literal[NodeType.CONDITION] = NodeType.CONDITION
    name: str
    args: list[Any] = Field(default_factory=list)


class DecoratorNode(BTNodeBase):
    type: Literal[NodeType.DECORATOR] = NodeType.DECORATOR
    kind: Literal["inverter", "repeater", "until_success"]
    child: "BTNodeType | None" = None
    times: int = 1
    max_tries: int = 10


class SubTreeNode(BTNodeBase):
    type: Literal[NodeType.SUBTREE] = NodeType.SUBTREE
    tree_id: str


class LLMNode(BTNodeBase):
    type: Literal[NodeType.LLM] = NodeType.LLM
    prompt: str
    choices: list[str] = Field(default_factory=list)
    expected: str | None = None


# Discriminated union keyed on ``type``.
BTNodeType = Annotated[
    Union[
        SequenceNode,
        SelectorNode,
        ActionNode,
        ConditionNode,
        DecoratorNode,
        SubTreeNode,
        LLMNode,
    ],
    Field(discriminator="type"),
]


_TYPE_MAP: dict[str, type[BTNodeBase]] = {
    NodeType.SEQUENCE.value: SequenceNode,
    NodeType.SELECTOR.value: SelectorNode,
    NodeType.ACTION.value: ActionNode,
    NodeType.CONDITION.value: ConditionNode,
    NodeType.DECORATOR.value: DecoratorNode,
    NodeType.SUBTREE.value: SubTreeNode,
    NodeType.LLM.value: LLMNode,
}


class BTTree(RootModel[Any]):
    """Top-level BT container — wraps a single root node.

    The serialized JSON shape puts node fields at the top level (e.g.
    ``{"id": "root", "type": "sequence", "children": [...]}``). ``model_validate``
    dispatches to the correct node class via the ``type`` discriminator and
    stores the validated node in ``.root``.

    Pydantic v2 RootModel does not natively support discriminated unions on the
    root payload, so we override ``model_validate`` to dispatch manually before
    delegating to the standard RootModel constructor.
    """

    @classmethod
    def model_validate(  # type: ignore[override]
        cls,
        data: Any,
        *,
        strict: bool | None = None,
        from_attributes: bool | None = None,
        context: Any | None = None,
    ) -> "BTTree":
        if isinstance(data, (SequenceNode, SelectorNode, ActionNode, ConditionNode,
                             DecoratorNode, SubTreeNode, LLMNode)):
            return cls(data)

        if isinstance(data, dict):
            ntype = data.get("type")
            if ntype is None:
                raise ValidationError.from_exception_data(
                    "BTTree",
                    [
                        {
                            "type": "missing",
                            "loc": ("type",),
                            "input": data,
                            "msg": "Field required",
                        }
                    ],
                )
            if ntype not in _TYPE_MAP:
                expected_str = ",".join(sorted(_TYPE_MAP.keys()))
                raise ValidationError.from_exception_data(
                    "BTTree",
                    [
                        {
                            "type": "enum",
                            "loc": ("type",),
                            "input": ntype,
                            "ctx": {"expected": expected_str},
                            "msg": f"Input should be one of: {expected_str}",
                        }
                    ],
                )
            node_cls = _TYPE_MAP[ntype]
            validated = node_cls.model_validate(
                data,
                strict=strict,
                from_attributes=from_attributes,
                context=context,
            )
            return cls(validated)

        # Other inputs: delegate to RootModel which raises ValidationError.
        return super().model_validate(
            data,
            strict=strict,
            from_attributes=from_attributes,
            context=context,
        )


# ``BTNode`` is a convenience alias matching the spec wording.
BTNode = BTTree

# Resolve forward references now that all node types are defined.
for _model in (
    SequenceNode,
    SelectorNode,
    ActionNode,
    ConditionNode,
    DecoratorNode,
    SubTreeNode,
    LLMNode,
):
    _model.model_rebuild()


__all__ = [
    "BTNode",
    "BTNodeBase",
    "BTTree",
    "NodeType",
    "Status",
    "SequenceNode",
    "SelectorNode",
    "ActionNode",
    "ConditionNode",
    "DecoratorNode",
    "SubTreeNode",
    "LLMNode",
]