import base64
import hashlib
import hmac
import json
import time
import uuid
from typing import Any

import pytest
from economy_service.app import app
from economy_service.db import get_pool
from economy_service.services import marketplace_service
from fastapi.testclient import TestClient

JWT_SECRET = "test-secret"
BUYER_ID = "11111111-1111-1111-1111-111111111111"
CREATOR_ID = "22222222-2222-2222-2222-222222222222"
ADMIN_ID = "33333333-3333-3333-3333-333333333333"
OCEAN = {"O": 0.7, "C": 0.8, "E": 0.5, "A": 0.6, "N": 0.3}
NPC_YAML = "saga:\n  name: welcome\n"


def auth_header(player_id: str) -> dict[str, str]:
    def encode(value: dict[str, Any]) -> str:
        raw = json.dumps(value, separators=(",", ":")).encode()
        return base64.urlsafe_b64encode(raw).rstrip(b"=").decode()

    header = encode({"alg": "HS256", "typ": "JWT"})
    payload = encode(
        {
            "sub": player_id,
            "uname": "e2e",
            "exp": int(time.time()) + 600,
        }
    )
    signature = hmac.new(
        JWT_SECRET.encode(),
        f"{header}.{payload}".encode(),
        hashlib.sha256,
    ).digest()
    signature_b64 = base64.urlsafe_b64encode(signature).rstrip(b"=").decode()
    return {"Authorization": f"Bearer {header}.{payload}.{signature_b64}"}


class FakeTransaction:
    async def __aenter__(self):
        return self

    async def __aexit__(self, *exc):
        return False


class FakeConnection:
    def __init__(self) -> None:
        self.players = {
            BUYER_ID: "player",
            CREATOR_ID: "creator",
            ADMIN_ID: "admin",
        }
        self.templates: dict[tuple[str, int], dict[str, Any]] = {}
        self.wallets = {BUYER_ID: 1000, CREATOR_ID: 0, ADMIN_ID: 0}
        self.purchases: dict[str, dict[str, Any]] = {}
        self.revenues: list[dict[str, Any]] = []
        self.transactions: list[tuple[Any, ...]] = []
        self.next_template_id = 41
        self.next_purchase_id = 71

    async def fetchrow(self, sql: str, *params: Any):
        if "FROM player WHERE id" in sql:
            role = self.players.get(params[0])
            return None if role is None else {"id": params[0], "role": role}
        if "FROM template_purchase WHERE idempotency_key" in sql:
            return self.purchases.get(params[0])
        if "FROM npc_template WHERE id" in sql:
            return self.templates.get(("npc_template", params[0]))
        if "FROM saga_template WHERE id" in sql:
            return self.templates.get(("saga_template", params[0]))
        if "INSERT INTO npc_template" in sql:
            return self._insert_template("npc_template", params)
        if "INSERT INTO saga_template" in sql:
            return self._insert_template("saga_template", params)
        if "INSERT INTO template_purchase" in sql:
            purchase_id = self.next_purchase_id
            self.next_purchase_id += 1
            purchase = {
                "id": purchase_id,
                "purchase_id": purchase_id,
                "user_id": params[0],
                "template_kind": params[1],
                "template_id": params[2],
                "price_paid_gold": params[3],
                "created_at": "2026-10-08T00:00:00Z",
            }
            self.purchases[params[4]] = purchase
            return {"id": purchase_id}
        if "FROM wallet WHERE user_id" in sql:
            if params[0] not in self.wallets:
                return None
            return {"gold_balance": self.wallets[params[0]], "token_balance": 0}
        return None

    async def execute(self, sql: str, *params: Any):
        if "UPDATE wallet SET gold_balance" in sql:
            self.wallets[params[1]] = params[0]
        elif "INSERT INTO creator_revenue" in sql:
            self.revenues.append(
                {
                    "creator_id": params[0],
                    "purchase_id": params[1],
                    "amount_gold": params[2],
                    "platform_cut_gold": 0,
                    "created_at": "2026-10-08T00:00:00Z",
                }
            )
        elif "INSERT INTO transaction" in sql:
            self.transactions.append(params)
        elif "UPDATE npc_template" in sql:
            self.templates[("npc_template", params[0])]["status"] = "taken_down"
        elif "UPDATE saga_template" in sql:
            self.templates[("saga_template", params[0])]["status"] = "taken_down"

    async def fetch(self, sql: str, *params: Any):
        if "FROM npc_template" in sql:
            rows = self._list_templates("npc_template", params[0])
        elif "FROM saga_template" in sql:
            rows = self._list_templates("saga_template", params[0])
        elif "FROM template_purchase" in sql:
            rows = [
                {key: value for key, value in purchase.items() if key != "id"}
                for purchase in self.purchases.values()
                if purchase["user_id"] == params[0]
            ]
        elif "FROM creator_revenue" in sql:
            rows = [
                {key: value for key, value in revenue.items() if key != "creator_id"}
                for revenue in self.revenues
                if revenue["creator_id"] == params[0]
            ]
        else:
            raise AssertionError(f"unexpected fetch: {sql}")
        return rows

    def _insert_template(self, table: str, params: tuple[Any, ...]):
        template_id = self.next_template_id
        self.next_template_id += 1
        template = {
            "id": template_id,
            "creator_id": params[0],
            "name": params[1],
            "status": "live",
            "created_at": "2026-10-08T00:00:00Z",
        }
        if table == "npc_template":
            template.update(price_gold=params[6])
        else:
            template.update(
                price_gold=params[2],
                npc_deps=params[6],
                semantic_version=params[7],
            )
        self.templates[(table, template_id)] = template
        return {"id": template_id}

    def _list_templates(self, table: str, status: str):
        return [
            dict(template)
            for (template_table, _), template in sorted(self.templates.items(), reverse=True)
            if template_table == table and template["status"] == status
        ]

    def transaction(self):
        return FakeTransaction()


class FakePool:
    def __init__(self, conn: FakeConnection):
        self._conn = conn

    def acquire(self):
        conn = self._conn

        class _ConnContext:
            async def __aenter__(self):
                return conn

            async def __aexit__(self, *exc):
                return False

        return _ConnContext()


@pytest.fixture
def client(monkeypatch):
    monkeypatch.setenv("JWT_SECRET", JWT_SECRET)
    conn = FakeConnection()
    pool = FakePool(conn)

    async def override_pool():
        return pool

    app.dependency_overrides[get_pool] = override_pool
    yield TestClient(app), conn
    app.dependency_overrides.clear()


@pytest.fixture(autouse=True)
def reset_marketplace_clients():
    marketplace_service.set_clients(kafka=None)
    yield
    marketplace_service.set_clients(kafka=None)
