# apps/economy-service/src/economy_service/db.py
"""asyncpg pool singleton."""
from __future__ import annotations
import os
import asyncpg

_pool: asyncpg.Pool | None = None


async def get_pool() -> asyncpg.Pool:
    global _pool
    if _pool is None:
        _pool = await asyncpg.create_pool(
            os.environ.get("DATABASE_URL",
                "postgresql://aicity:aicity@postgres:5432/aicity"),
            min_size=1,
            max_size=10,
        )
    return _pool


async def close_pool() -> None:
    global _pool
    if _pool is not None:
        await _pool.close()
        _pool = None
