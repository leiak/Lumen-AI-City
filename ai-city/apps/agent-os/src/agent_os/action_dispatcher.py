"""NPC action 执行层（Sprint 12 min slice）。

只暴露 say()，行为：构造 envelope → Redis publish → 失败仅 warn（fire-and-forget）。

Sprint 13+ 才会加 move() / wait() / give()。
"""
from __future__ import annotations

import json
import logging
import time
import uuid
from typing import Any, Protocol

logger = logging.getLogger(__name__)


class _RedisLike(Protocol):
    async def publish(self, channel: str, payload: str) -> None: ...


class ActionDispatcher:
    def __init__(self, redis: _RedisLike, *, channel: str) -> None:
        self._redis = redis
        self._channel = channel

    async def say(
        self,
        npc_id: str,
        text: str,
        *,
        player_id: str | None = None,
        tile_id: str | None = None,
        options: list[dict[str, Any]] | None = None,
        reply_to_choice_id: str | None = None,
    ) -> None:
        """发送一句 NPC 台词。失败仅 warn，agent tick 不阻塞。

        envelope:
        {
          "type": "npc_dialogue",
          "trace_id": "<uuid>",
          "ts_ms": <int ms>,
          "payload": {
            "npc_id": "...", "player_id": "...", "tile_id": "...",
            "say": "...",
            "options": [...],                   // 始终是数组（不是 null）
            "reply_to_choice_id": "..."|null    // null = 主动 say；非空 = 玩家回复
          }
        }
        """
        envelope: dict[str, Any] = {
            "type": "npc_dialogue",
            "trace_id": str(uuid.uuid4()),
            "ts_ms": int(time.time() * 1000),
            "payload": {
                "npc_id": npc_id,
                "player_id": player_id or "",
                "tile_id": tile_id or "",
                "say": text,
                "options": list(options) if options else [],
                "reply_to_choice_id": reply_to_choice_id,
            },
        }
        try:
            await self._redis.publish(self._channel, json.dumps(envelope, ensure_ascii=False))
        except Exception as e:  # noqa: BLE001
            logger.warning(
                "dispatcher.say publish failed",
                extra={"npc_id": npc_id, "err": str(e)},
            )