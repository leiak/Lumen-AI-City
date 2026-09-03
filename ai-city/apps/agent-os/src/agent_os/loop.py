"""Agent OS 五模块循环。

Tick + Event 混合驱动（§B.1）。
详细见 docs/05-Agent-OS.md §19.1-§19.14。
"""
from __future__ import annotations

import asyncio
import logging
import time
import uuid
from collections.abc import Awaitable, Callable
from dataclasses import dataclass, field
from typing import Any

logger = logging.getLogger(__name__)


@dataclass
class Percept:
    """感知输入：来自 Tick 或 Event。"""

    source: str  # "tick" | "player" | "npc" | "federation"
    kind: str    # "movement" | "dialogue" | "story_event" | ...
    payload: dict[str, Any]
    priority: int = 0
    ts_ms: int = field(default_factory=lambda: int(time.time() * 1000))


@dataclass
class Decision:
    """决策输出：行动指令。"""

    action: str
    args: dict[str, Any]
    confidence: float = 1.0
    why: str = ""
    trace_id: str = field(default_factory=lambda: str(uuid.uuid4()))


class AgentRuntime:
    """单个 Agent 的运行时。

    五模块：Perception → Planning → Action → Reflection → Memory。
    """

    def __init__(
        self,
        agent_id: str,
        tick_seconds: float = 5.0,
        planner: Callable[[list[Percept], Decision | None], Awaitable[Decision]] | None = None,
        actor: Callable[[Decision], Awaitable[None]] | None = None,
    ) -> None:
        self.agent_id = agent_id
        self.tick_interval = tick_seconds
        self.high_pri_queue: asyncio.Queue[Percept] = asyncio.Queue()
        self._stop = asyncio.Event()
        self._last_decision: Decision | None = None
        self.planner = planner or self._default_planner
        self.actor = actor or self._default_actor

    async def push_percept(self, percept: Percept) -> None:
        """外部推入感知事件（来自 WebSocket / Kafka 等）。"""
        if percept.priority >= 8:  # 高优直接走打断队列
            await self.high_pri_queue.put(percept)
        else:
            # TODO: 累积到 tick 批
            pass

    async def run(self) -> None:
        """主循环：每个 tick 处理一批 percept + 立即处理紧急事件。"""
        logger.info(f"agent {self.agent_id} running")
        while not self._stop.is_set():
            try:
                batch = await self._gather_batch()
                urgent = await self._drain_high_priority()
                decision = await self.planner(batch + urgent, self._last_decision)
                if decision:
                    await self.actor(decision)
                    self._last_decision = decision
                await asyncio.sleep(self.tick_interval)
            except Exception:
                logger.exception(f"agent {self.agent_id} loop error")
                await asyncio.sleep(self.tick_interval)

    async def stop(self) -> None:
        self._stop.set()

    async def _gather_batch(self) -> list[Percept]:
        """累积 tick 期间的 percept（合并为一次 LLM 调用以省 token）。"""
        return []  # TODO: 从内部缓冲拉取

    async def _drain_high_priority(self) -> list[Percept]:
        """拉取紧急队列（不等 tick）。"""
        urgent: list[Percept] = []
        while not self.high_pri_queue.empty():
            urgent.append(self.high_pri_queue.get_nowait())
        return urgent

    async def _default_planner(
        self, percepts: list[Percept], last: Decision | None
    ) -> Decision | None:
        """默认占位：仅在没有 percept 时跳过。"""
        if not percepts:
            return None
        return Decision(
            action="noop",
            args={"reason": "default planner"},
            why="no LLM configured",
        )

    async def _default_actor(self, decision: Decision) -> None:
        """默认占位：仅记录日志。"""
        logger.info(f"agent {self.agent_id} execute {decision.action} {decision.args}")
