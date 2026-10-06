# apps/economy-service/tests/conftest.py
"""Test fixtures: in-memory asyncpg pool + redis fake."""
from __future__ import annotations

from unittest.mock import AsyncMock, MagicMock

import pytest
import pytest_asyncio


@pytest_asyncio.fixture
async def pool():
    """Provide mocked asyncpg pool backed by in-memory dicts.

    asyncpg's pool.acquire() returns an async context manager that yields a
    connection. conn.transaction() also returns an async context manager
    directly (not a coroutine).
    """
    pool = MagicMock()
    conn = AsyncMock()

    # In-memory store: user_id -> (gold, token)
    wallets: dict[str, tuple[int, int]] = {}

    async def fetchrow_wallet(user_id):
        if user_id not in wallets:
            return None
        g, t = wallets[user_id]
        return {"user_id": user_id, "gold_balance": g, "token_balance": t,
                "created_at": "2026-10-06T00:00:00Z",
                "updated_at": "2026-10-06T00:00:00Z"}

    async def fetchrow_default(*args, **kwargs):
        # Simple dispatch based on SQL fragment
        if "SELECT" in args[0] and "FROM wallet" in args[0]:
            return await fetchrow_wallet(args[1] if len(args) > 1 else None)
        return None

    async def execute_default(*args, **kwargs):
        # Handle INSERT into wallet (idempotent create-or-find)
        if args and "INSERT INTO wallet" in args[0]:
            uid = args[1]
            if uid not in wallets:
                wallets[uid] = (0, 0)
            return "INSERT 0 1"
        # Handle UPDATE wallet — mutate the in-memory store
        if args and "UPDATE wallet SET" in args[0]:
            sql = args[0]
            new_balance = args[1]
            user_id = args[2]
            g, t = wallets.get(user_id, (0, 0))
            if "gold_balance" in sql:
                g = new_balance
            if "token_balance" in sql:
                t = new_balance
            wallets[user_id] = (g, t)
            return "UPDATE 1"
        # UPDATE product SET stock = stock - 1 (W2.3 purchase)
        if args and "UPDATE product SET stock" in args[0]:
            return "UPDATE 1"
        # INSERT INTO transaction — append-only log; no-op for in-memory mock
        return "OK"

    # Use AsyncMock so tests can override .side_effect with a list of return values.
    # When not overridden, dispatch via the default callable above.
    conn.fetchrow = AsyncMock(side_effect=fetchrow_default)
    conn.execute = AsyncMock(side_effect=execute_default)

    # conn.transaction() returns an async CM directly (matches real asyncpg)
    class _TransactionCtx:
        async def __aenter__(self):
            return self

        async def __aexit__(self, *exc):
            return False

    conn.transaction = MagicMock(return_value=_TransactionCtx())

    pool.acquire.return_value.__aenter__ = AsyncMock(return_value=conn)
    pool.acquire.return_value.__aexit__ = AsyncMock(return_value=False)

    # Expose store for tests
    pool._wallets = wallets
    pool._conn = conn
    return pool


@pytest.fixture
def redis_client():
    """Fake Redis with SET NX + GET + TTL."""
    r = MagicMock()
    store: dict[str, str] = {}
    r._store = store

    async def set_nx(key, value):
        if key in store:
            return False
        store[key] = value
        return True

    async def get(key):
        return store.get(key)

    r.set = AsyncMock(side_effect=set_nx)
    r.get = AsyncMock(side_effect=get)
    return r
