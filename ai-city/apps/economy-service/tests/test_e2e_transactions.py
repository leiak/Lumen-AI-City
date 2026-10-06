"""E2E test for /api/v1/transactions/{user_id} endpoint."""
from __future__ import annotations
from datetime import datetime
from unittest.mock import AsyncMock, patch

import pytest
from fastapi.testclient import TestClient

from economy_service.app import app


@pytest.fixture
def client(pool):
    with patch("economy_service.app.get_pool", return_value=pool):
        with patch("economy_service.app.start_scheduler", return_value=AsyncMock()):
            with patch("economy_service.api.v1.transactions.get_pool", return_value=pool):
                with TestClient(app) as c:
                    yield c


def test_list_transactions_returns_history(client, pool):
    """Endpoint returns transaction rows + total + pagination."""
    # Mock fetch() returning 2 fake rows
    fake_rows = [
        {
            "id": 1, "tx_type": "player_transfer", "currency": "gold",
            "amount": -100, "balance_after": 9900, "counterparty_id": "bob",
            "product_id": None, "trace_id": None, "created_at": datetime(2026, 10, 6, 12, 0, 0),
        },
        {
            "id": 2, "tx_type": "npc_purchase", "currency": "gold",
            "amount": -50, "balance_after": 9850, "counterparty_id": None,
            "product_id": 1, "trace_id": "bt_xyz", "created_at": datetime(2026, 10, 6, 12, 5, 0),
        },
    ]
    pool._conn.fetch = AsyncMock(return_value=fake_rows)
    pool._conn.fetchval = AsyncMock(return_value=2)

    r = client.get("/api/v1/transactions/alice?limit=10")
    assert r.status_code == 200
    body = r.json()
    assert body["total"] == 2
    assert body["limit"] == 10
    assert body["offset"] == 0
    assert len(body["transactions"]) == 2
    assert body["transactions"][0]["tx_type"] == "player_transfer"
    assert body["transactions"][1]["product_id"] == 1
    assert body["transactions"][0]["created_at"] == "2026-10-06T12:00:00"


def test_list_transactions_empty_history(client, pool):
    """Empty history → transactions=[], total=0."""
    pool._conn.fetch = AsyncMock(return_value=[])
    pool._conn.fetchval = AsyncMock(return_value=0)
    r = client.get("/api/v1/transactions/alice")
    assert r.status_code == 200
    body = r.json()
    assert body["transactions"] == []
    assert body["total"] == 0


def test_list_transactions_pagination_params(client, pool):
    """limit=5&offset=10 → passes through to SQL."""
    pool._conn.fetch = AsyncMock(return_value=[])
    pool._conn.fetchval = AsyncMock(return_value=0)
    r = client.get("/api/v1/transactions/alice?limit=5&offset=10")
    assert r.status_code == 200
    body = r.json()
    assert body["limit"] == 5
    assert body["offset"] == 10
    # Verify fetch was called with the right pagination args
    args, _ = pool._conn.fetch.call_args
    assert args[1] == "alice"  # user_id
    assert args[2] == 5  # limit
    assert args[3] == 10  # offset