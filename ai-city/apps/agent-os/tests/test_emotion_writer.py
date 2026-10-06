"""MemoryWriter emotion extension tests."""
from __future__ import annotations
import json
from unittest.mock import AsyncMock, MagicMock

import pytest


@pytest.mark.asyncio
async def test_write_emotions_inserts_one_row_per_emotion():
    from agent_os.memory.writer import MemoryWriter
    # pool is MagicMock so pool.acquire() returns its return_value (AsyncMock
    # context manager) directly, instead of a coroutine. This matches
    # asyncpg.Pool.acquire, which is a synchronous-return async context mgr.
    pool = MagicMock()
    pool.acquire.return_value = AsyncMock()
    pool.acquire.return_value.__aenter__.return_value = AsyncMock()
    conn = pool.acquire.return_value.__aenter__.return_value
    writer = MemoryWriter(pool)

    await writer.write_emotions(
        npc_id="npc_a_wang", player_id="p1", session_id="sess-1",
        emotions=["happy", "neutral", "happy"],
    )

    # Verify executemany was called with 3 rows
    assert conn.executemany.called
    call_args = conn.executemany.call_args
    assert "INSERT INTO memory_player_session" in call_args[0][0]
    rows = call_args[0][1]
    assert len(rows) == 3
    assert rows[0][3] == "happy"  # emotion is 4th positional


@pytest.mark.asyncio
async def test_write_emotions_empty_list_noop():
    from agent_os.memory.writer import MemoryWriter
    pool = MagicMock()
    writer = MemoryWriter(pool)

    await writer.write_emotions(
        npc_id="npc_a_wang", player_id="p1", session_id="sess-1", emotions=[],
    )

    # No DB call
    assert not pool.acquire.called


@pytest.mark.asyncio
async def test_write_emotions_swallows_exceptions():
    """Best-effort: writer must not raise even if PG is down."""
    from agent_os.memory.writer import MemoryWriter
    pool = AsyncMock()
    pool.acquire.side_effect = RuntimeError("PG down")
    writer = MemoryWriter(pool)

    # Should NOT raise
    await writer.write_emotions(
        npc_id="npc_a_wang", player_id="p1", session_id="sess-1",
        emotions=["happy"],
    )
