import time
import uuid
from unittest.mock import AsyncMock, MagicMock

import pytest
from economy_service.app import app
from economy_service.db import get_pool
from economy_service.errors import (
    InsufficientBalance,
    NoWithdrawableRevenueError,
    SelfPurchaseError,
    TemplateNotFoundError,
    TemplateTakenDownError,
)
from fastapi.testclient import TestClient

JWT_SECRET = "test-secret"
BUYER_ID = str(uuid.uuid4())
CREATOR_ID = str(uuid.uuid4())


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
    yield TestClient(app), conn, monkeypatch
    app.dependency_overrides.clear()


def patch_service(monkeypatch, method_name, result=None, error=None):
    class FakeMarketplaceService:
        def __init__(self, pool):
            self.pool = pool

        async def fake_call(*args, **kwargs):
            if error is not None:
                raise error
            return result

    setattr(
        FakeMarketplaceService,
        method_name,
        AsyncMock(side_effect=FakeMarketplaceService.fake_call),
    )
    monkeypatch.setattr(
        "economy_service.api.v1.marketplace.purchase.MarketplaceService",
        FakeMarketplaceService,
    )
    return FakeMarketplaceService


def test_purchase_endpoint_maps_success(client):
    test_client, conn, monkeypatch = client
    conn.fetchrow.return_value = {"id": BUYER_ID, "role": "player"}
    fake_service = patch_service(monkeypatch, "purchase_template", result=71)

    response = test_client.post(
        "/v1/marketplace/purchase",
        json={"template_kind": "saga", "template_id": 43, "idempotency_key": "key-1"},
        headers=auth_header(BUYER_ID),
    )

    assert response.status_code == 200
    assert response.json() == {"purchase_id": 71}
    fake_service.purchase_template.assert_awaited_once_with(
        user_id=BUYER_ID,
        template_kind="saga",
        template_id=43,
        idempotency_key="key-1",
    )


@pytest.mark.parametrize(
    "error,status_code,code",
    [
        (InsufficientBalance("no gold"), 402, "R_022"),
        (TemplateNotFoundError("missing"), 404, "R_028"),
        (TemplateTakenDownError("gone"), 410, "R_029"),
        (SelfPurchaseError("self"), 403, "R_033"),
    ],
)
def test_purchase_endpoint_maps_domain_errors(client, error, status_code, code):
    test_client, conn, monkeypatch = client
    conn.fetchrow.return_value = {"id": BUYER_ID, "role": "player"}
    patch_service(monkeypatch, "purchase_template", error=error)

    response = test_client.post(
        "/v1/marketplace/purchase",
        json={"template_kind": "npc", "template_id": 42, "idempotency_key": "key-1"},
        headers=auth_header(BUYER_ID),
    )

    assert response.status_code == status_code
    assert response.json()["detail"]["code"] == code


def test_inventory_requires_authentication(client):
    test_client, _, _ = client

    response = test_client.get(f"/v1/marketplace/inventory/{BUYER_ID}")

    assert response.status_code == 401


def test_inventory_allows_owner(client):
    test_client, conn, monkeypatch = client
    conn.fetchrow.return_value = {"id": BUYER_ID, "role": "player"}
    fake_service = patch_service(
        monkeypatch, "list_inventory", result=[{"purchase_id": 71, "template_kind": "npc"}]
    )

    response = test_client.get(
        f"/v1/marketplace/inventory/{BUYER_ID}", headers=auth_header(BUYER_ID)
    )

    assert response.status_code == 200
    assert response.json() == [{"purchase_id": 71, "template_kind": "npc"}]
    fake_service.list_inventory.assert_awaited_once_with(BUYER_ID, limit=50, offset=0)


def test_inventory_rejects_other_player(client):
    test_client, conn, monkeypatch = client
    conn.fetchrow.return_value = {"id": BUYER_ID, "role": "player"}
    patch_service(monkeypatch, "list_inventory", result=[])

    response = test_client.get(
        f"/v1/marketplace/inventory/{CREATOR_ID}", headers=auth_header(BUYER_ID)
    )

    assert response.status_code == 403
    assert response.json()["detail"]["code"] == "R_026"


def test_revenue_allows_creator_owner(client):
    test_client, conn, monkeypatch = client
    conn.fetchrow.return_value = {"id": CREATOR_ID, "role": "creator"}
    fake_service = patch_service(
        monkeypatch, "list_creator_revenue", result=[{"purchase_id": 71, "amount_gold": 100}]
    )

    response = test_client.get(
        f"/v1/marketplace/revenue/{CREATOR_ID}", headers=auth_header(CREATOR_ID)
    )

    assert response.status_code == 200
    assert response.json() == [{"purchase_id": 71, "amount_gold": 100}]
    fake_service.list_creator_revenue.assert_awaited_once_with(CREATOR_ID, limit=50, offset=0)


def test_revenue_rejects_creator_for_other_creator(client):
    test_client, conn, monkeypatch = client
    conn.fetchrow.return_value = {"id": BUYER_ID, "role": "creator"}
    patch_service(monkeypatch, "list_creator_revenue", result=[])

    response = test_client.get(
        f"/v1/marketplace/revenue/{CREATOR_ID}", headers=auth_header(BUYER_ID)
    )

    assert response.status_code == 403
    assert response.json()["detail"]["code"] == "R_027"


def test_revenue_summary_allows_creator_owner(client):
    test_client, conn, monkeypatch = client
    conn.fetchrow.return_value = {"id": CREATOR_ID, "role": "creator"}
    summary = {"earned_gold": 100, "withdrawn_gold": 30, "available_gold": 70}
    fake_service = patch_service(
        monkeypatch, "get_creator_revenue_summary", result=summary
    )

    response = test_client.get(
        f"/v1/marketplace/revenue/{CREATOR_ID}/summary",
        headers=auth_header(CREATOR_ID),
    )

    assert response.status_code == 200
    assert response.json() == summary
    fake_service.get_creator_revenue_summary.assert_awaited_once_with(CREATOR_ID)


def test_revenue_withdraw_allows_creator_owner(client):
    test_client, conn, monkeypatch = client
    conn.fetchrow.return_value = {"id": CREATOR_ID, "role": "creator"}
    result = {
        "withdrawal_id": 9,
        "amount_gold": 70,
        "balance_after": 10,
        "status": "settled",
    }
    fake_service = patch_service(
        monkeypatch, "withdraw_creator_revenue", result=result
    )

    response = test_client.post(
        f"/v1/marketplace/revenue/{CREATOR_ID}/withdraw",
        json={"idempotency_key": "creator-withdraw-1"},
        headers=auth_header(CREATOR_ID),
    )

    assert response.status_code == 200
    assert response.json() == result
    fake_service.withdraw_creator_revenue.assert_awaited_once_with(
        creator_id=CREATOR_ID,
        idempotency_key="creator-withdraw-1",
    )


def test_revenue_withdraw_rejects_other_creator(client):
    test_client, conn, monkeypatch = client
    conn.fetchrow.return_value = {"id": BUYER_ID, "role": "creator"}
    patch_service(monkeypatch, "withdraw_creator_revenue", result={})

    response = test_client.post(
        f"/v1/marketplace/revenue/{CREATOR_ID}/withdraw",
        json={"idempotency_key": "creator-withdraw-1"},
        headers=auth_header(BUYER_ID),
    )

    assert response.status_code == 403
    assert response.json()["detail"]["code"] == "R_027"


def test_revenue_withdraw_maps_zero_available(client):
    test_client, conn, monkeypatch = client
    conn.fetchrow.return_value = {"id": CREATOR_ID, "role": "creator"}
    patch_service(
        monkeypatch,
        "withdraw_creator_revenue",
        error=NoWithdrawableRevenueError("no revenue"),
    )

    response = test_client.post(
        f"/v1/marketplace/revenue/{CREATOR_ID}/withdraw",
        json={"idempotency_key": "creator-withdraw-2"},
        headers=auth_header(CREATOR_ID),
    )

    assert response.status_code == 400
    assert response.json()["detail"]["code"] == "R_035"
