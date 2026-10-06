"""BT evaluator (Phase C.1).

Recursive ``tick(node, state)`` that walks the tree and dispatches to actions,
conditions, and decorators. Enforces ``max_ticks`` infinite-loop protection
by raising ``BTError`` when exceeded.

Composite nodes (sequence / selector) short-circuit on FAILURE or RUNNING.
Leaf nodes dispatch to the action / condition registry. Decorator nodes
recurse into the child then transform the returned status. SubTree nodes
look up the referenced tree in the ``BTTreeRegistry`` and re-tick. LLM nodes
use a deterministic v0 stub based on ``expected`` + ``prompt`` keyword match.
"""
from __future__ import annotations

from agent_os.bt.actions import dispatch_action
from agent_os.bt.conditions import dispatch_condition
from agent_os.bt.decorators import tick_decorator
from agent_os.bt.errors import BTError
from agent_os.bt.registry import BTTreeRegistry
from agent_os.bt.schema import (
    ActionNode,
    BTTree,
    ConditionNode,
    DecoratorNode,
    LLMNode,
    SelectorNode,
    SequenceNode,
    Status,
    SubTreeNode,
)
from agent_os.bt.state import BTState


def _llm_stub(node: LLMNode) -> Status:
    """v0 deterministic LLM stub.

    Returns SUCCESS if ``expected`` appears as a keyword in ``prompt`` (case
    insensitive), else FAILURE. This is a placeholder — C.4 wires the real
    LLM client and is gated by ``BT_RUNTIME_ENABLED`` kill switch.
    """
    if node.expected is None:
        return Status.FAILURE
    return Status.SUCCESS if node.expected.lower() in node.prompt.lower() else Status.FAILURE


def tick(tree: BTTree, state: BTState) -> Status:
    """Tick the root node once. Returns ``Status``.

    Enforces ``max_ticks``: each call to ``tick`` (root or recursive) increments
    ``state.tick_count`` and raises ``BTError`` when exceeded.
    """
    state.tick_count += 1
    if state.tick_count > state.max_ticks:
        raise BTError(f"max_ticks ({state.max_ticks}) exceeded at node {tree.root.id}")
    return _tick_node(tree.root, state)


def _tick_node(node: object, state: BTState) -> Status:
    """Recursively tick a single node (does not increment tick_count)."""
    if isinstance(node, SequenceNode):
        for child in node.children:
            status = tick_child(child, state)
            if status != Status.SUCCESS:
                return status  # propagate FAILURE or RUNNING
        return Status.SUCCESS

    if isinstance(node, SelectorNode):
        for child in node.children:
            status = tick_child(child, state)
            if status != Status.FAILURE:
                return status  # propagate SUCCESS or RUNNING
        return Status.FAILURE

    if isinstance(node, ActionNode):
        return dispatch_action(node.name, node.args, state)

    if isinstance(node, ConditionNode):
        result = dispatch_condition(node.name, node.args, state)
        return Status.SUCCESS if result else Status.FAILURE

    if isinstance(node, DecoratorNode):
        # Schema validation ensures node.child is not None at parse-time;
        # tick_decorator also defensively re-checks for safety.
        return tick_decorator(node, state, tick_child)

    if isinstance(node, SubTreeNode):
        if state.registry is None:
            raise BTError(
                f"subtree {node.tree_id!r} requires BTState.registry to be set"
            )
        sub = state.registry.get(node.tree_id)
        # SubTreeNode is a routing hop — the parent's tick_child(SubTreeNode)
        # already accounted for one tick. Recurse into the subtree's root via
        # _tick_node (no extra increment) so the subtree's root + its
        # children consume ticks just like any leaf sibling at the parent
        # level. Routing through ``tick`` or ``tick_child`` here would add
        # a phantom +1 that makes SubTreeNode cost 2 vs leaf's 1.
        # max_ticks is still enforced because the subtree's own children are
        # ticked via ``tick_child`` (sequence / selector children) and any
        # nested SubTreeNode re-enters this same routing branch.
        return _tick_node(sub.root, state)

    if isinstance(node, LLMNode):
        return _llm_stub(node)

    raise BTError(f"unknown node type: {type(node).__name__}")


def tick_child(node: object, state: BTState) -> Status:
    """Tick a child node, enforcing max_ticks.

    Each child tick increments the global counter so a runaway deep tree
    triggers ``BTError`` before it can hang the runtime.
    """
    state.tick_count += 1
    if state.tick_count > state.max_ticks:
        raise BTError(f"max_ticks ({state.max_ticks}) exceeded")
    return _tick_node(node, state)


__all__ = ["BTTreeRegistry", "tick", "tick_child"]