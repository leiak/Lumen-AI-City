"""Integration test for admin endpoints."""
from __future__ import annotations
from unittest.mock import AsyncMock, patch

import pytest
from fastapi.testclient import TestClient

from economy_service.app import app


@pytest.fixture
def client(pool):
    with patch("economy_service.app.get_pool", return_value=pool):
        with patch("economy_service.app.start_scheduler", return_value=AsyncMock()):
            with patch("economy_service.api.v1.admin.get_pool", return_value=pool):
                with TestClient(app) as c:
                    yield c


def test_admin_emit_requires_token(client, pool):
    r = client.post("/api/v1/admin/central-bank/emit")
    assert r.status_code == 403
    assert r.json()["detail"]["code"] == "R_026"


def test_admin_emit_invalid_token(client):
    """Invalid Bearer token → 403 / R_026."""
    r = client.post(
        "/api/v1/admin/central-bank/emit",
        headers={"Authorization": "Bearer wrong-token"},
    )
    assert r.status_code == 403
    assert r.json()["detail"]["code"] == "R_026"


def test_admin_emit_succeeds_with_token(client, pool):
    pool._wallets["alice"] = (100, 0)
    pool._conn.fetchval.side_effect = [1, 100]  # 1 active, total 100
    r = client.post(
        "/api/v1/admin/central-bank/emit",
        headers={"Authorization": "Bearer dev-admin-token"},
    )
    assert r.status_code == 200


def test_admin_sink_succeeds_with_token(client, pool):
    """Admin sink deducts gold from user; 200 with {user_id, sunk, balance_after}."""
    pool._wallets["alice"] = (500, 0)
    # sink() calls conn.fetchval once: SELECT gold_balance FROM wallet ... FOR UPDATE
    pool._conn.fetchval.return_value = 500
    # conn.execute default side_effect handles UPDATE wallet SET (mutates _wallets)
    # and INSERT INTO central_bank_ledger (returns "OK" — append-only).
    r = client.post(
        "/api/v1/admin/central-bank/sink",
        json={"user_id": "alice", "amount": 50},
        headers={"Authorization": "Bearer dev-admin-token"},
    )
    assert r.status_code == 200
    body = r.json()
    assert body["user_id"] == "alice"
    assert body["sunk"] == 50
    assert body["balance_after"] == 450
    # in-memory store updated
    assert pool._wallets["alice"] == (450, 0)
