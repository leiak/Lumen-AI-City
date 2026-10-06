"""Integration test for wallet endpoints (FastAPI TestClient + mocked pool)."""
from __future__ import annotations
from unittest.mock import AsyncMock, patch

import pytest
from fastapi.testclient import TestClient

from economy_service.app import app


@pytest.fixture
def client(pool):
    """TestClient with mocked pool + mocked Redis."""
    fake_redis = AsyncMock()
    fake_redis.set = AsyncMock(return_value=True)
    fake_redis.get = AsyncMock(return_value=None)

    with patch("economy_service.api.v1.wallet.get_pool", return_value=pool), \
         patch("economy_service.app.get_pool", return_value=pool), \
         patch("economy_service.api.v1.wallet._redis", return_value=fake_redis):
        with TestClient(app) as c:
            yield c


def test_get_wallet_404_returns_r_018(client, pool):
    """Missing wallet → 404 with R_018 code."""
    r = client.get("/api/v1/wallet/unknown-user")
    assert r.status_code == 404
    assert r.json()["detail"]["code"] == "R_018"


def test_get_wallet_returns_balances(client, pool):
    pool._wallets["alice"] = (1000, 100)
    r = client.get("/api/v1/wallet/alice")
    assert r.status_code == 200
    body = r.json()
    assert body["user_id"] == "alice"
    assert body["gold_balance"] == 1000
    assert body["token_balance"] == 100


def test_transfer_endpoint_succeeds(client, pool):
    pool._wallets["alice"] = (1000, 0)
    pool._wallets["bob"] = (0, 0)
    r = client.post("/api/v1/wallet/transfer", json={
        "from_user_id": "alice",
        "to_user_id": "bob",
        "currency": "gold",
        "amount": 100,
        "idempotency_key": "e2e-key-1",
    })
    assert r.status_code == 200
    assert r.json()["gold_balance"] == 900


def test_transfer_insufficient_returns_402(client, pool):
    pool._wallets["alice"] = (50, 0)
    r = client.post("/api/v1/wallet/transfer", json={
        "from_user_id": "alice",
        "to_user_id": "bob",
        "currency": "gold",
        "amount": 100,
        "idempotency_key": "e2e-key-2",
    })
    assert r.status_code == 402
    assert r.json()["detail"]["code"] == "R_022"