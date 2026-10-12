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


def test_create_saga_template_creates_draft(client):
    test_client, conn = client
    conn.fetchrow.side_effect = [
        {"id": CREATOR_ID, "role": "creator"},
        {"id": 41},
    ]

    response = test_client.post(
        "/v1/marketplace/saga-templates",
        json={
            "name": "Draft Saga",
            "yaml_content": VALID_YAML,
            "price_gold": 100,
            "semantic_version": "1.0.0",
            "status": "draft",
        },
        headers=auth_header(CREATOR_ID),
    )

    assert response.status_code == 201
    assert response.json() == {"id": 41}
    assert conn.fetchrow.call_args.args[-1] == "draft"


def test_saga_drafts_requires_authentication(client):
    test_client, _ = client

    response = test_client.get("/v1/marketplace/saga-templates/drafts")

    assert response.status_code == 401


def test_saga_drafts_returns_owner_drafts(client):
    test_client, conn = client
    conn.fetchrow.return_value = {"id": CREATOR_ID, "role": "creator"}
    conn.fetch.return_value = [{"id": 41, "status": "draft", "name": "Draft Saga"}]

    response = test_client.get(
        "/v1/marketplace/saga-templates/drafts",
        headers=auth_header(CREATOR_ID),
    )

    assert response.status_code == 200
    assert response.json() == [{"id": 41, "status": "draft", "name": "Draft Saga"}]
    assert "creator_id = $2" in conn.fetch.call_args.args[0]
    assert conn.fetch.call_args.args[2] == CREATOR_ID


def test_saga_works_returns_owner_works_across_states(client):
    test_client, conn = client
    conn.fetchrow.return_value = {"id": CREATOR_ID, "role": "creator"}
    conn.fetch.return_value = [
        {"id": 41, "status": "draft", "name": "Draft Saga"},
        {"id": 42, "status": "live", "name": "Live Saga"},
        {"id": 43, "status": "taken_down", "name": "Paused Saga"},
    ]

    response = test_client.get(
        "/v1/marketplace/saga-templates/works",
        headers=auth_header(CREATOR_ID),
    )

    assert response.status_code == 200
    assert len(response.json()) == 3
    assert "WHERE creator_id = $1" in conn.fetch.call_args.args[0]
    assert conn.fetch.call_args.args[1] == CREATOR_ID


def test_publish_saga_draft_allows_owner(client):
    test_client, conn = client
    conn.fetchrow.side_effect = [
        {"id": CREATOR_ID, "role": "creator"},
        {"creator_id": CREATOR_ID, "status": "draft"},
    ]

    response = test_client.post(
        "/v1/marketplace/saga-templates/41/publish",
        headers=auth_header(CREATOR_ID),
    )

    assert response.status_code == 200
    assert response.json() == {"status": "live"}


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
