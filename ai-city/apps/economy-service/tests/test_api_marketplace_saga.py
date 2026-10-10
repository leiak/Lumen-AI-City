import time
import uuid
from unittest.mock import AsyncMock, MagicMock

import pytest
from economy_service.app import app
from economy_service.db import get_pool
from fastapi.testclient import TestClient

JWT_SECRET = "test-secret"
CREATOR_ID = str(uuid.uuid4())
ADMIN_ID = str(uuid.uuid4())
VALID_YAML = "saga:\n  name: welcome\n"


def auth_header(sub: str):
    import base64
    import hashlib
    import hmac
    import json

    def encode(value: dict) -> str:
        raw = json.dumps(value, separators=(",", ":")).encode()
        return base64.urlsafe_b64encode(raw).rstrip(b"=").decode()

    header = encode({"alg": "HS256", "typ": "JWT"})
    payload = encode({"sub": sub, "uname": "demo", "exp": int(time.time()) + 600})
    signature = hmac.new(JWT_SECRET.encode(), f"{header}.{payload}".encode(), hashlib.sha256).digest()
    signature_b64 = base64.urlsafe_b64encode(signature).rstrip(b"=").decode()
    return {"Authorization": f"Bearer {header}.{payload}.{signature_b64}"}


@pytest.fixture
def client(monkeypatch):
    monkeypatch.setenv("JWT_SECRET", JWT_SECRET)
    conn = AsyncMock()
    pool = MagicMock()
    pool.acquire.return_value.__aenter__ = AsyncMock(return_value=conn)
    pool.acquire.return_value.__aexit__ = AsyncMock(return_value=False)

    async def override_pool():
        return pool

    app.dependency_overrides[get_pool] = override_pool
    yield TestClient(app), conn
    app.dependency_overrides.clear()


def test_create_saga_template_validates_yaml(client):
    test_client, conn = client
    conn.fetchrow.side_effect = [
        {"id": CREATOR_ID, "role": "creator"},
        {"id": 41},
    ]

    response = test_client.post(
        "/v1/marketplace/saga-templates",
        json={
            "name": "Broken Saga",
            "yaml_content": "name: 'foo",
            "price_gold": 100,
            "semantic_version": "1.0.0",
        },
        headers=auth_header(CREATOR_ID),
    )

    assert response.status_code == 400
    assert response.json()["detail"]["code"] == "R_032"


def test_create_saga_template_returns_id(client):
    test_client, conn = client
    conn.fetchrow.side_effect = [
        {"id": CREATOR_ID, "role": "creator"},
        {"id": 41},
    ]

    response = test_client.post(
        "/v1/marketplace/saga-templates",
        json={
            "name": "Welcome Saga",
            "yaml_content": VALID_YAML,
            "price_gold": 100,
            "semantic_version": "1.0.0",
            "npc_deps": ["npc_wang_boss_001"],
        },
        headers=auth_header(CREATOR_ID),
    )

    assert response.status_code == 201
    assert response.json() == {"id": 41}


def test_get_saga_template_returns_not_found(client):
    test_client, conn = client
    conn.fetchrow.return_value = None

    response = test_client.get("/v1/marketplace/saga-templates/99")

    assert response.status_code == 404
    assert response.json()["detail"]["code"] == "R_028"


def test_take_down_saga_requires_creator_or_admin(client):
    test_client, conn = client
    conn.fetchrow.return_value = {"id": CREATOR_ID, "role": "user"}

    response = test_client.post(
        "/v1/marketplace/saga-templates/41/take-down",
        headers=auth_header(CREATOR_ID),
    )

    assert response.status_code == 403
    assert response.json()["detail"]["code"] == "R_027"


def test_take_down_saga_allows_owner(client):
    test_client, conn = client
    conn.fetchrow.side_effect = [
        {"id": CREATOR_ID, "role": "creator"},
        {"id": 41, "creator_id": CREATOR_ID, "status": "live"},
    ]

    response = test_client.post(
        "/v1/marketplace/saga-templates/41/take-down",
        headers=auth_header(CREATOR_ID),
    )

    assert response.status_code == 200
    assert response.json() == {"status": "taken_down"}


def test_take_down_saga_allows_admin(client):
    test_client, conn = client
    conn.fetchrow.side_effect = [
        {"id": ADMIN_ID, "role": "admin"},
        {"id": 41, "creator_id": CREATOR_ID, "status": "live"},
    ]

    response = test_client.post(
        "/v1/marketplace/saga-templates/41/take-down",
        headers=auth_header(ADMIN_ID),
    )

    assert response.status_code == 200
    assert response.json() == {"status": "taken_down"}
