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


def test_admin_emit_succeeds_with_token(client, pool):
    pool._wallets["alice"] = (100, 0)
    pool._conn.fetchval.side_effect = [1, 100]  # 1 active, total 100
    r = client.post(
        "/api/v1/admin/central-bank/emit",
        headers={"Authorization": "Bearer dev-admin-token"},
    )
    assert r.status_code == 200
