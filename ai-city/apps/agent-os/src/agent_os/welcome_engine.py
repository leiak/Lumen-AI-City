"""事件驱动 welcome：玩家首入 NPC home tile 触发一次 say.welcome（Sprint 13）。

上游：redis_sub 订阅 `aicity:player:moved` → 逐条 payload 喂 handle_payload。
逻辑：
- 先更新 PlayerListener；
- 玩家从未被 welcome 过时，对所有 home tile 匹配的 enabled NPC 各喊一句
  welcome（缺 welcome 时回退 greeting），并携带 initial 节点 options 供点选；
- 只在真正喊过话后才标记 welcomed（该次事件匹配到 NPC）；
  之后该玩家再进 home tile 不再触发。

与 say_scheduler.py 的定时问候不互斥：没有玩家在场时仍由 tick 兜底。
"""
from __future__ import annotations

import logging
import random
from typing import Protocol

from agent_os.npc_registry import NpcRegistry
from agent_os.player_listener import PlayerListener, parse_player_moved
from agent_os.say_scheduler import _root_options

logger = logging.getLogger(__name__)


class _DispatcherLike(Protocol):
    async def say(
        self,
        npc_id: str,
        text: str,
        *,
        player_id: str | None = None,
        tile_id: str | None = None,
        options: list[dict[str, str]] | None = None,
    ) -> None: ...


class WelcomeEngine:
    def __init__(
        self,
        *,
        registry: NpcRegistry,
        dispatcher: _DispatcherLike,
        listener: PlayerListener | None = None,
    ) -> None:
        self._registry = registry
        self._dispatcher = dispatcher
        self._listener = listener or PlayerListener()
        self._rng = random.Random()

    @property
    def listener(self) -> PlayerListener:
        return self._listener

    async def handle_payload(self, payload: str) -> None:
        """解析一条 player:moved 并触发 welcome（如有匹配的 NPC）。"""
        pos = parse_player_moved(payload)
        if pos is None:
            return
        self._listener.update_position(pos)
        if self._listener.is_welcomed(pos.player_id):
            return
        spoke = False
        for tpl in self._registry.list_enabled():
            if not tpl.home_tile_id or pos.tile_id != tpl.home_tile_id:
                continue
            lines = tpl.say.welcome or tpl.say.greeting
            if not lines:
                lines = [tpl.talk_tree.default_say] if tpl.talk_tree.default_say else []
            if not lines:
                continue
            text = self._rng.choice(lines)
            options = _root_options(tpl)
            try:
                await self._dispatcher.say(
                    tpl.npc_id,
                    text,
                    player_id=pos.player_id,
                    tile_id=pos.tile_id,
                    options=options,
                )
                spoke = True
            except Exception as e:  # noqa: BLE001
                logger.warning("welcome say failed", extra={"npc_id": tpl.npc_id, "err": str(e)})
        if spoke:
            self._listener.mark_welcomed(pos.player_id)
