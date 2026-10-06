"""Integration test for purchase endpoint."""
from __future__ import annotations
from unittest.mock import AsyncMock, patch

import pytest
from fastapi.testclient import TestClient

from economy_service.app import app


@pytest.fixture
def client(pool):
    fake_redis = AsyncMock()
    fake_redis.set = AsyncMock(return_value=True)
    fake_redis.get = AsyncMock(return_value=None)

    with patch("economy_service.api.v1.wallet.get_pool", return_value=pool):
        with patch("economy_service.api.v1.products.get_pool", return_value=pool):
            with patch("economy_service.api.v1.wallet._redis", return_value=fake_redis):
                with patch("economy_service.app.get_pool", return_value=pool):
                    with TestClient(app) as c:
                        yield c


def test_list_products_for_npc(client, pool):
    # Mock list query
    pool._conn.fetch.return_value = [
        {"id": 1, "npc_id": "npc_wang_boss_001", "name": "招牌红烧肉",
         "price_gold": 50, "price_token": None, "stock": 10},
    ]
    r = client.get("/api/v1/products/npc_wang_boss_001")
    assert r.status_code == 200
    body = r.json()
    assert len(body) == 1
    assert body[0]["name"] == "招牌红烧肉"


def test_purchase_endpoint_returns_402_on_insufficient(client, pool):
    pool._wallets["alice"] = (10, 0)
    pool._conn.fetchrow.side_effect = [
        {"id": 1, "npc_id": "npc_wang_boss_001", "name": "招牌红烧肉",
         "price_gold": 50, "price_token": None, "stock": 10, "enabled": True},
        {"bal": 10},
    ]
    r = client.post("/api/v1/wallet/purchase", json={
        "user_id": "alice", "product_id": 1, "currency": "gold",
        "idempotency_key": "e2e-purchase-key-1",
    })
    assert r.status_code == 402
    assert r.json()["detail"]["code"] == "R_022"
