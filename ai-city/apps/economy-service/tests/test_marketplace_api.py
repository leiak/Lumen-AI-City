import base64
import hashlib
import hmac
import json
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
OCEAN = {"O": 0.7, "C": 0.8, "E": 0.5, "A": 0.6, "N": 0.3}


def encode(value: dict) -> str:
    raw = json.dumps(value, separators=(",", ":")).encode()
    return base64.urlsafe_b64encode(raw).rstrip(b"=").decode()


def make_jwt(sub: str, exp_offset: int = 600) -> str:
    header = encode({"alg": "HS256", "typ": "JWT"})
    payload = encode({"sub": sub, "uname": "demo", "exp": int(time.time()) + exp_offset})
    signing_input = f"{header}.{payload}".encode()
    signature = hmac.new(JWT_SECRET.encode(), signing_input, hashlib.sha256).digest()
    signature_b64 = base64.urlsafe_b64encode(signature).rstrip(b"=").decode()
    return f"{header}.{payload}.{signature_b64}"


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
    yield TestClient(app), conn, pool
    app.dependency_overrides.clear()


def auth_header(sub: str, exp_offset: int = 600):
    return {"Authorization": f"Bearer {make_jwt(sub, exp_offset)}"}


def set_player_role(conn, role: str):
    conn.fetchrow.side_effect = [
        {"id": CREATOR_ID if role != "admin" else ADMIN_ID, "role": role},
        {"id": 21},
    ]


def test_post_npc_template_requires_creator_role(client):
    test_client, conn, _ = client
    set_player_role(conn, "player")

    response = test_client.post(
        "/v1/marketplace/npc-templates",
        json={"name": "Chef", "ocean_json": OCEAN, "price_gold": 100},
        headers=auth_header(CREATOR_ID),
    )

    assert response.status_code == 403
    assert response.json()["detail"]["code"] == "R_027"


def test_post_npc_template_rejects_invalid_bt(client):
    test_client, conn, _ = client
    set_player_role(conn, "creator")

    response = test_client.post(
        "/v1/marketplace/npc-templates",
        json={
            "name": "Chef",
            "ocean_json": OCEAN,
            "price_gold": 100,
            "bt_skeleton": "{",
        },
        headers=auth_header(CREATOR_ID),
    )

    assert response.status_code == 400
    assert response.json()["detail"]["code"] == "R_031"


def test_post_npc_template_creates_template(client):
    test_client, conn, _ = client
    set_player_role(conn, "creator")

    response = test_client.post(
        "/v1/marketplace/npc-templates",
        json={"name": "Chef", "ocean_json": OCEAN, "price_gold": 100},
        headers=auth_header(CREATOR_ID),
    )

    assert response.status_code == 201
    assert response.json() == {"id": 21}


def test_get_npc_template_returns_not_found(client):
    test_client, conn, _ = client
    conn.fetchrow.return_value = None

    response = test_client.get("/v1/marketplace/npc-templates/99")

    assert response.status_code == 404
    assert response.json()["detail"]["code"] == "R_028"


def test_take_down_requires_admin_role(client):
    test_client, conn, _ = client
    conn.fetchrow.side_effect = [{"id": CREATOR_ID, "role": "creator"}]

    response = test_client.post(
        "/v1/marketplace/npc-templates/21/take-down",
        headers=auth_header(CREATOR_ID),
    )

    assert response.status_code == 403
    assert response.json()["detail"]["code"] == "R_026"


def test_take_down_allows_admin_role(client):
    test_client, conn, _ = client
    conn.fetchrow.side_effect = [{"id": ADMIN_ID, "role": "admin"}]

    response = test_client.post(
        "/v1/marketplace/npc-templates/21/take-down",
        headers=auth_header(ADMIN_ID),
    )

    assert response.status_code == 200
    assert response.json() == {"status": "taken_down"}
