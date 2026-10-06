"""Emotion repository integration tests with dev PG."""
from __future__ import annotations

import os
from datetime import datetime, timedelta, timezone

import asyncpg
import pytest
import pytest_asyncio

from agent_os.emotion.repository import EmotionRepository


DSN = os.getenv("TEST_PG_DSN", "postgresql://aicity:aicity_dev@localhost:5432/aicity")
TEST_NPC = "test_npc_emotion_repo"


@pytest_asyncio.fixture
async def pg_pool():
    pool = await asyncpg.create_pool(DSN, min_size=1, max_size=2)
    yield pool
    # Cleanup
    async with pool.acquire() as conn:
        await conn.execute("DELETE FROM memory_player_session WHERE npc_id = $1", TEST_NPC)
    await pool.close()


@pytest.mark.asyncio
async def test_fetch_player_distribution_filters_by_player(pg_pool):
    repo = EmotionRepository(pg_pool)
    now = datetime.now(timezone.utc)
    async with pg_pool.acquire() as conn:
        await conn.execute(
            """INSERT INTO memory_player_session (npc_id, player_id, message, emotion, created_at)
               VALUES ($1, $2, $3::jsonb, $4, $5)""",
            TEST_NPC, "p1", '{"type":"emotion"}', "happy", now - timedelta(minutes=10),
        )
        await conn.execute(
            """INSERT INTO memory_player_session (npc_id, player_id, message, emotion, created_at)
               VALUES ($1, $2, $3::jsonb, $4, $5)""",
            TEST_NPC, "p2", '{"type":"emotion"}', "sad", now - timedelta(minutes=10),
        )

    dist = await repo.fetch_player_distribution(TEST_NPC, "p1")
    assert dist.weights.get("happy", 0) > 0
    assert "sad" not in dist.weights  # p2's sad filtered out


@pytest.mark.asyncio
async def test_fetch_global_distribution_ignores_player(pg_pool):
    repo = EmotionRepository(pg_pool)
    now = datetime.now(timezone.utc)
    async with pg_pool.acquire() as conn:
        for p in ("p1", "p2", "p3"):
            await conn.execute(
                """INSERT INTO memory_player_session (npc_id, player_id, message, emotion, created_at)
                   VALUES ($1, $2, $3::jsonb, $4, $5)""",
                TEST_NPC, p, '{"type":"emotion"}', "happy", now - timedelta(minutes=5),
            )

    dist = await repo.fetch_global_distribution(TEST_NPC)
    assert dist.total_rows == 3
    assert dist.weights.get("happy") == 1.0


@pytest.mark.asyncio
async def test_fetch_player_distribution_skips_null_emotion(pg_pool):
    repo = EmotionRepository(pg_pool)
    now = datetime.now(timezone.utc)
    async with pg_pool.acquire() as conn:
        # NULL emotion row (pre-migration backfill simulation)
        await conn.execute(
            """INSERT INTO memory_player_session (npc_id, player_id, message, created_at)
               VALUES ($1, $2, $3::jsonb, $4)""",
            TEST_NPC, "p1", '{"type":"emotion"}', now - timedelta(minutes=10),
        )
        await conn.execute(
            """INSERT INTO memory_player_session (npc_id, player_id, message, emotion, created_at)
               VALUES ($1, $2, $3::jsonb, $4, $5)""",
            TEST_NPC, "p1", '{"type":"emotion"}', "happy", now - timedelta(minutes=5),
        )

    dist = await repo.fetch_player_distribution(TEST_NPC, "p1")
    assert dist.total_rows == 1  # NULL skipped
    assert dist.weights == {"happy": 1.0}


@pytest.mark.asyncio
async def test_env_tau_override(pg_pool, monkeypatch):
    monkeypatch.setenv("EMOTION_TAU_PLAYER_SECONDS", "60")  # 1 minute
    repo = EmotionRepository(pg_pool)
    assert repo._tau_player == 60.0


@pytest.mark.asyncio
async def test_env_limit_override(pg_pool, monkeypatch):
    monkeypatch.setenv("EMOTION_PLAYER_LIMIT", "5")
    repo = EmotionRepository(pg_pool)
    assert repo._player_limit == 5
