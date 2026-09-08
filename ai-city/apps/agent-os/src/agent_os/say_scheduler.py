"""每 tick 给 list_enabled() 里每个 NPC 选 1 句 greeting 调 dispatcher.say。

Sprint 12 min slice：纯定时器，不感知 player listener。
Sprint 13+ 会替换为事件驱动（玩家进入 tile_0_0 才触发）。
"""
from __future__ import annotations

import asyncio
import logging
import random
from typing import Protocol

from agent_os.npc_registry import NpcRegistry

logger = logging.getLogger(__name__)


class _DispatcherLike(Protocol):
    async def say(self, npc_id: str, text: str, **kwargs) -> None: ...


class SayScheduler:
    def __init__(
        self,
        *,
        registry: NpcRegistry,
        dispatcher: _DispatcherLike,
        tick_seconds: float,
    ) -> None:
        self._registry = registry
        self._dispatcher = dispatcher
        self._tick = tick_seconds
        self._rng = random.Random()

    async def tick_once(self) -> None:
        for tpl in self._registry.list_enabled():
            greetings = tpl.say.greeting
            if not greetings:
                continue
            text = self._rng.choice(greetings)
            try:
                await self._dispatcher.say(tpl.npc_id, text)
            except Exception as e:  # noqa: BLE001
                logger.warning("scheduler tick failed", extra={"npc_id": tpl.npc_id, "err": str(e)})

    async def run(self, stop: asyncio.Event) -> None:
        """长寿命 task。stop.set() 后下一次 tick 退出。"""
        logger.info("say_scheduler starting", extra={"tick_seconds": self._tick})
        while not stop.is_set():
            try:
                await self.tick_once()
            except Exception as e:  # noqa: BLE001
                logger.exception("scheduler tick exception", extra={"err": str(e)})
            try:
                await asyncio.wait_for(stop.wait(), timeout=self._tick)
            except asyncio.TimeoutError:
                pass
        logger.info("say_scheduler stopped")