"""每 tick 给 list_enabled() 里每个 NPC 选 1 句 greeting 调 dispatcher.say。

Sprint 12 min slice：纯定时器，不感知 player listener。
Sprint 13：接入 PlayerListener —— 该 NPC home tile 已有玩家在场时 skip 一次
（进场 welcome 由 WelcomeEngine 负责，tick 不叠加广播，避免同一对话框双说）。
"""
from __future__ import annotations

import asyncio
import logging
import random
from typing import Protocol

from agent_os.npc_registry import NpcRegistry

logger = logging.getLogger(__name__)


def _root_options(tpl) -> list[dict[str, str]] | None:
    """返回对话树入口节点(initial)的选项，供主动 say 携带；无树/无选项时返回 None。"""
    initial = tpl.talk_tree.initial
    if not initial:
        return None
    node = tpl.talk_tree.nodes.get(initial)
    if not node or not node.options:
        return None
    return [{"id": o.id, "text": o.text} for o in node.options]


class _DispatcherLike(Protocol):
    async def say(self, npc_id: str, text: str, **kwargs) -> None: ...


class _PlayerListenerLike(Protocol):
    def players_in_tile(self, tile_id: str) -> list: ...


class SayScheduler:
    def __init__(
        self,
        *,
        registry: NpcRegistry,
        dispatcher: _DispatcherLike,
        tick_seconds: float,
        listener: _PlayerListenerLike | None = None,
    ) -> None:
        self._registry = registry
        self._dispatcher = dispatcher
        self._tick = tick_seconds
        self._listener = listener
        self._rng = random.Random()

    def _has_player_at_home(self, tpl) -> bool:
        """该 NPC home tile 当前是否有玩家在场。"""
        if self._listener is None or not tpl.home_tile_id:
            return False
        return bool(self._listener.players_in_tile(tpl.home_tile_id))

    async def tick_once(self) -> None:
        for tpl in self._registry.list_enabled():
            greetings = tpl.say.greeting
            if not greetings:
                continue
            # Sprint 13：home tile 有玩家时，进场问候交给 WelcomeEngine（一次性
            # welcome），tick 不再广播 —— 避免玩家在场时 welcome 与 greeting 叠加。
            if self._has_player_at_home(tpl):
                continue
            text = self._rng.choice(greetings)
            options = _root_options(tpl)
            try:
                await self._dispatcher.say(tpl.npc_id, text, options=options)
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