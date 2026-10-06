"""PG repository for emotion history aggregation."""
from __future__ import annotations

import os
from datetime import datetime, timezone

import asyncpg

from .aggregate import EmotionDistribution, EmotionRow, aggregate_distribution


class EmotionRepository:
    """Reads memory_player_session and computes emotion distributions.

    Settings via env vars (see settings module for canonical list):
      - EMOTION_TAU_PLAYER_SECONDS (default 7200)
      - EMOTION_TAU_GLOBAL_SECONDS (default 86400)
      - EMOTION_PLAYER_LIMIT (default 50)
      - EMOTION_GLOBAL_LIMIT (default 100)
    """

    def __init__(self, pool: asyncpg.Pool):
        self._pool = pool
        self._tau_player = float(os.getenv("EMOTION_TAU_PLAYER_SECONDS", "7200"))
        self._tau_global = float(os.getenv("EMOTION_TAU_GLOBAL_SECONDS", "86400"))
        self._player_limit = int(os.getenv("EMOTION_PLAYER_LIMIT", "50"))
        self._global_limit = int(os.getenv("EMOTION_GLOBAL_LIMIT", "100"))

    async def fetch_player_distribution(
        self, npc_id: str, player_id: str,
    ) -> EmotionDistribution:
        rows = await self._pool.fetch(
            """
            SELECT emotion, created_at FROM memory_player_session
            WHERE npc_id = $1 AND player_id = $2
              AND emotion IS NOT NULL
            ORDER BY created_at DESC
            LIMIT $3
            """,
            npc_id, player_id, self._player_limit,
        )
        return aggregate_distribution(
            (EmotionRow(emotion=r["emotion"], created_at=_ensure_tz(r["created_at"]))
             for r in rows),
            tau_seconds=self._tau_player,
        )

    async def fetch_global_distribution(
        self, npc_id: str,
    ) -> EmotionDistribution:
        rows = await self._pool.fetch(
            """
            SELECT emotion, created_at FROM memory_player_session
            WHERE npc_id = $1
              AND emotion IS NOT NULL
            ORDER BY created_at DESC
            LIMIT $2
            """,
            npc_id, self._global_limit,
        )
        return aggregate_distribution(
            (EmotionRow(emotion=r["emotion"], created_at=_ensure_tz(r["created_at"]))
             for r in rows),
            tau_seconds=self._tau_global,
        )


def _ensure_tz(dt: datetime) -> datetime:
    """asyncpg returns naive datetimes; attach UTC tzinfo."""
    if dt.tzinfo is None:
        return dt.replace(tzinfo=timezone.utc)
    return dt
