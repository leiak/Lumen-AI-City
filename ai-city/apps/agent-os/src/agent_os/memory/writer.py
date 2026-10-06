"""MemoryWriter: persist NPC conversation turns + emotion tags to PostgreSQL.

Best-effort writes: failures are logged but never raised — the user-visible
stream must not break because persistence is degraded.
"""
from __future__ import annotations

import json
import logging

import asyncpg

_logger = logging.getLogger(__name__)


class MemoryWriter:
    """Persists NPC session turns (including per-beat emotion tags) to PG."""

    def __init__(self, pool: asyncpg.Pool):
        self._pool = pool

    async def write_emotions(
        self,
        *,
        npc_id: str,
        player_id: str,
        session_id: str,
        emotions: list[str],
    ) -> None:
        """Persist emotion tags as one row each in memory_player_session.

        Best-effort: any exception (PG down, etc.) is logged and swallowed —
        emotion persistence must never break the user-visible stream.

        If `emotions` is empty, this is a no-op (no DB call).
        """
        if not emotions:
            return
        try:
            async with self._pool.acquire() as conn:
                await conn.executemany(
                    """
                    INSERT INTO memory_player_session
                        (npc_id, player_id, message, emotion)
                    VALUES ($1, $2, $3::jsonb, $4)
                    """,
                    [
                        (npc_id, player_id,
                         json.dumps({"session_id": session_id, "type": "emotion"}),
                         emo)
                        for emo in emotions if emo
                    ],
                )
        except Exception as e:
            _logger.warning(
                "emotion persist failed (npc=%s session=%s): %s",
                npc_id, session_id, e,
            )
