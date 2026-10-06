"""BT runtime state — a mutable dataclass passed through ticks.

BTState carries all the side-effect outputs of running a behavior tree:
``npc_move_target``, ``say_buffer``, ``wait_until``, ``npc_state``,
``player_position``, ``time_of_day``. The evaluator reads player position +
npc_state when evaluating conditions; writes ``npc_move_target`` +
``say_buffer`` + ``wait_until`` + ``npc_state`` when running actions.
``tick_count`` tracks how many ticks have been issued to enforce ``max_ticks``
infinite-loop protection.
"""
from __future__ import annotations

from dataclasses import dataclass, field
from typing import TYPE_CHECKING, Any

from agent_os.bt.errors import BTError

if TYPE_CHECKING:
    from agent_os.bt.registry import BTTreeRegistry


@dataclass
class BTState:
    """Mutable state passed to every tick. See module docstring."""

    tick_count: int = 0
    max_ticks: int = 100
    player_position: tuple[int, int] | None = None
    npc_state: dict[str, Any] = field(default_factory=dict)
    npc_move_target: tuple[int, int] | None = None
    say_buffer: list[dict[str, Any]] = field(default_factory=list)
    wait_until: float | None = None
    time_of_day: str = "noon"
    registry: "BTTreeRegistry | None" = None


__all__ = ["BTError", "BTState"]