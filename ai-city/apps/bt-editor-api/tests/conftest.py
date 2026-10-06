"""Test fixtures — mock asyncpg pool + FastAPI TestClient.

Phase C.2 bt-editor-api: tests MUST NOT require live PG. We patch the
module-level pool singleton in ``bt_editor_api.db`` with an ``AsyncMock``
so endpoints can be exercised end-to-end via FastAPI's ``TestClient``.

Usage::

    def test_x(mock_pool, client):
        mock_pool.fetch.return_value = [...]
        resp = client.get("/api/v1/bt/npc_a")
"""
from __future__ import annotations

from unittest.mock import AsyncMock

import pytest
from bt_editor_api import db
from bt_editor_api.app import app
from fastapi.testclient import TestClient


@pytest.fixture
def mock_pool() -> AsyncMock:
    """AsyncMock that quacks like an ``asyncpg.Pool`` for the duration of one test."""
    pool = AsyncMock()
    pool.fetch.return_value = []  # default: empty result set
    pool.fetchrow.return_value = None  # default: no row found
    pool.fetchval.return_value = None
    pool.execute.return_value = "INSERT 0 1"
    db.set_pool(pool)
    yield pool
    db.set_pool(None)


@pytest.fixture
def client() -> TestClient:
    """FastAPI TestClient — NOTE: not used as a context manager so lifespan
    is NOT triggered (we never touch a real PG)."""
    return TestClient(app)
