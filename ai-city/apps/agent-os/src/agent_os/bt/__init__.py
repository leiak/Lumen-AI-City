"""BT runtime — Phase C.1 public exports.

Behavior tree runtime in pure Python: 7 node types (sequence / selector /
action / condition / decorator / subtree / llm), 5 actions, 3 conditions,
3 decorators. ``tick(tree, state)`` walks the tree recursively with
``max_ticks=100`` infinite-loop protection.
"""
from __future__ import annotations

from agent_os.bt.actions import (
    ACTION_REGISTRY,
    dispatch_action,
    move_to_tile,
    noop,
    say_to_player,
    set_npc_state,
    wait,
)
from agent_os.bt.conditions import (
    CONDITION_REGISTRY,
    dispatch_condition,
    npc_state_equals,
    player_in_tile,
    time_of_day_in,
)
from agent_os.bt.decorators import (
    DECORATOR_REGISTRY,
    apply_decorator,
    inverter,
    repeater,
    tick_decorator,
    until_success,
)
from agent_os.bt.errors import BTError
from agent_os.bt.evaluator import BTTreeRegistry, tick, tick_child
from agent_os.bt.loader import load_tree, loads_tree
from agent_os.bt.schema import (
    BTNode,
    BTNodeBase,
    BTTree,
    LLMNode,
    NodeType,
    Status,
)
from agent_os.bt.state import BTState

__all__ = [
    "ACTION_REGISTRY",
    "BTError",
    "BTNode",
    "BTNodeBase",
    "BTTree",
    "BTTreeRegistry",
    "CONDITION_REGISTRY",
    "DECORATOR_REGISTRY",
    "LLMNode",
    "NodeType",
    "Status",
    "BTState",
    "apply_decorator",
    "dispatch_action",
    "dispatch_condition",
    "inverter",
    "load_tree",
    "loads_tree",
    "move_to_tile",
    "noop",
    "npc_state_equals",
    "player_in_tile",
    "repeater",
    "say_to_player",
    "set_npc_state",
    "tick",
    "tick_child",
    "tick_decorator",
    "time_of_day_in",
    "until_success",
    "wait",
]