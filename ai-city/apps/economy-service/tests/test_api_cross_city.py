from __future__ import annotations

from datetime import UTC, datetime, timedelta
from unittest.mock import AsyncMock, MagicMock
from uuid import uuid4

import pytest
from economy_service.app import app
from economy_service.auth import AuthenticatedPlayer, get_current_player
from economy_service.db import get_pool
from fastapi.testclient import TestClient

from tests.test_cross_city_service import FakeConn, transfer_row


def pool_for(conn):
    pool = MagicMock()
    pool.acquire.return_value.__aenter__ = AsyncMock(return_value=conn)
    pool.acquire.return_value.__aexit__ = AsyncMock(return_value=False)
    return pool


def setup_client(conn, user_id, role="player"):
    pool = pool_for(conn)

    async def override_pool():
        return pool

    def override_player():
        return AuthenticatedPlayer(id=user_id, username="demo", role=role)

    app.dependency_overrides[get_pool] = override_pool
    app.dependency_overrides[get_current_player] = override_player
    return TestClient(app)


def teardown_client():
    app.dependency_overrides.clear()


def body(**overrides):
    values = {
        "source_city_id": "alpha",
        "source_user_id": "alice",
        "destination_city_id": "beta",
        "destination_user_id": "bob",
        "currency": "gold",
        "amount": 100,
        "idempotency_key": "api-cross-key",
        "trace_id": "trace-api",
    }
    values.update(overrides)
    return values


@pytest.fixture(autouse=True)
def reset_overrides():
    yield
    app.dependency_overrides.clear()


def test_reserve_requires_matching_source_user():
    conn = FakeConn()
    client = setup_client(conn, "mallory")
    response = client.post("/api/v1/cross-city-transfers", json=body())

    assert response.status_code == 403
    assert response.json()["detail"]["code"] == "CROSS_CITY_VALIDATION_FAILED"
    assert conn.calls == []


def test_reserve_returns_201():
    conn = FakeConn()
    client = setup_client(conn, "alice")
    response = client.post("/api/v1/cross-city-transfers", json=body())

    assert response.status_code == 201
    data = response.json()
    assert data["status"] == "reserved"
    assert data["amount"] == 100
    assert conn.wallet_balance == 400


def test_reserve_maps_service_error():
    conn = FakeConn()
    conn.wallet_balance = 50
    client = setup_client(conn, "alice")
    response = client.post("/api/v1/cross-city-transfers", json=body())

    assert response.status_code == 402
    assert response.json()["detail"]["code"] == "R_022"


def test_get_allows_source_owner():
    conn = FakeConn()
    row = transfer_row()
    conn.rows.append(row)
    client = setup_client(conn, "alice")
    response = client.get(f"/api/v1/cross-city-transfers/{row['global_id']}")

    assert response.status_code == 200
    assert response.json()["global_id"] == str(row["global_id"])


def test_get_rejects_unrelated_player():
    conn = FakeConn()
    row = transfer_row()
    conn.rows.append(row)
    client = setup_client(conn, "mallory")
    response = client.get(f"/api/v1/cross-city-transfers/{row['global_id']}")

    assert response.status_code == 403


def test_get_404():
    client = setup_client(FakeConn(), "alice")
    response = client.get(f"/api/v1/cross-city-transfers/{uuid4()}")

    assert response.status_code == 404
    assert response.json()["detail"]["code"] == "CROSS_CITY_SOURCE_NOT_FOUND"


def test_refund_requires_admin():
    conn = FakeConn()
    conn.rows.append(
        transfer_row(expires_at=datetime.now(UTC) - timedelta(seconds=1))
    )
    client = setup_client(conn, "alice", role="player")
    response = client.post(f"/api/v1/cross-city-transfers/{conn.rows[0]['global_id']}/refund")

    assert response.status_code == 403


def test_refund_allows_admin():
    conn = FakeConn()
    row = transfer_row(expires_at=datetime.now(UTC) - timedelta(seconds=1))
    conn.rows.append(row)
    conn.bridge_balance = row["amount"]
    client = setup_client(conn, "admin", role="admin")
    response = client.post(f"/api/v1/cross-city-transfers/{conn.rows[0]['global_id']}/refund")

    assert response.status_code == 200
    assert response.json()["status"] == "refunded"
    assert conn.wallet_balance == 600
