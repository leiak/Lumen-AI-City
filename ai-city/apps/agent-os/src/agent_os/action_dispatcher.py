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

        Publishes the INNER payload only (no outer envelope) — ws-gateway wraps
        it uniformly with type/trace_id/ts_ms before fanning out to the browser.
        This matches the world-engine pattern for `aicity:player:moved`.

        payload:
        {
          "npc_id": "...", "player_id": "...", "tile_id": "...",
          "say": "...",
          "options": [...],                   // 始终是数组（不是 null）
          "reply_to_choice_id": "..."|null,   // null = 主动 say；非空 = 玩家回复
          "ts_ms": <int ms>,
          "trace_id": "<uuid>"
        }
        """
        payload: dict[str, Any] = {
            "npc_id": npc_id,
            "player_id": player_id or "",
            "tile_id": tile_id or "",
            "say": text,
            "options": list(options) if options else [],
            "reply_to_choice_id": reply_to_choice_id,
            "ts_ms": int(time.time() * 1000),
            "trace_id": str(uuid.uuid4()),
        }
        try:
            await self._redis.publish(self._channel, json.dumps(payload, ensure_ascii=False))
        except Exception as e:  # noqa: BLE001
            logger.warning(
                "dispatcher.say publish failed",
                extra={"npc_id": npc_id, "err": str(e)},
            )