#!/usr/bin/env python3
"""GA golden-path E2E suite (20 cases)."""

from __future__ import annotations

import os
import sys
import time
import uuid

import httpx

API = os.environ.get("AICITY_API_URL", "http://localhost:18080")
DEMO_USER = os.environ.get("AICITY_DEMO_USER", "demo")
DEMO_PASS = os.environ.get("AICITY_DEMO_PASS", "demo123")
CREATOR_USER = os.environ.get("AICITY_CREATOR_USER", "creator_demo")
CREATOR_PASS = os.environ.get("AICITY_CREATOR_PASS", "creatorpass")


class Suite:
    def __init__(self) -> None:
        self.client = httpx.Client(base_url=API, timeout=10.0)
        self.passed = 0
        self.failed = 0
        self.run_id = uuid.uuid4().hex[:8]
        self.token = ""
        self.player_id = ""

    def check(self, name: str, status: int, expected: int, body=None, extra=None) -> None:
        ok = status == expected
        if ok and extra is not None:
            ok = extra
        if ok:
            self.passed += 1
            print(f"  [PASS] {name} ({status})")
        else:
            self.failed += 1
            print(f"  [FAIL] {name}: status={status} expected={expected} body={body!r}")

    def auth_headers(self) -> dict[str, str]:
        return {"Authorization": f"Bearer {self.token}"}


def main() -> int:
    suite = Suite()
    c = suite.client
    suffix = suite.run_id
    new_user = f"ga_e2e_{suffix}"
    new_email = f"ga_e2e_{suffix}@aicity.dev"
    new_pass = "ga-e2e-pass-123"

    print(f"==> Golden-path E2E against {API}")

    r = c.get("/health")
    suite.check("01 health", r.status_code, 200, r.text, r.json().get("status") == "ok")

    r = c.get("/metrics")
    suite.check("02 metrics", r.status_code, 200, r.text)

    r = c.post(
        "/v1/auth/register",
        json={
            "username": new_user,
            "email": new_email,
            "password": new_pass,
            "display_name": "GA E2E",
        },
    )
    suite.check(
        "03 register",
        r.status_code,
        201,
        r.text,
        r.json().get("player_id") is not None,
    )

    r = c.post(
        "/v1/auth/login",
        json={"username": new_user, "password": new_pass},
    )
    body = r.json() if r.status_code == 200 else {}
    suite.token = body.get("token", "")
    suite.player_id = body.get("player_id", "")
    suite.check(
        "04 login",
        r.status_code,
        200,
        r.text,
        bool(suite.token and suite.player_id),
    )

    r = c.get("/v1/tiles")
    suite.check("05 tiles", r.status_code, 200, r.text)

    r = c.post(
        "/v1/npc/npc_wang_boss_001/talk",
        json={"npc_id": "npc_wang_boss_001", "player_id": "guest_e2e", "choice_id": "ask_food"},
    )
    body = r.json() if r.status_code == 200 else {}
    suite.check(
        "06 npc talk",
        r.status_code,
        200,
        r.text,
        body.get("npc_id") == "npc_wang_boss_001",
    )

    products = []
    for npc_id in ("npc_wang_boss_001", "npc_grace_healer_001", "npc_snack_owner_001", "npc_book_keeper_001"):
        r = c.get(f"/v1/products/{npc_id}")
        if r.status_code == 200 and isinstance(r.json(), list):
            products.extend(r.json())
    suite.check(
        "07 product list",
        r.status_code,
        200,
        r.text,
        isinstance(products, list) and len(products) > 0,
    )

    r = c.get("/v1/marketplace/npc-templates")
    suite.check("08 npc template list", r.status_code, 200, r.text)

    r = c.get("/v1/marketplace/saga-templates")
    suite.check("09 saga template list", r.status_code, 200, r.text)

    r = c.post(
        "/v1/world/move",
        json={
            "player_id": f"ga_e2e_mover_{suffix}",
            "from_tile_id": "tile_0_0",
            "to_tile_id": "tile_0_1",
            "x": 10,
            "y": 110,
        },
    )
    suite.check("10 world move", r.status_code, 200, r.text)

    r = c.get("/v1/players/me", headers=suite.auth_headers())
    suite.check("11 players/me", r.status_code, 200, r.text)

    r = c.post(
        "/v1/auth/login",
        json={"username": DEMO_USER, "password": DEMO_PASS},
    )
    demo_token = r.json().get("token", "") if r.status_code == 200 else ""
    demo_player_id = r.json().get("player_id", "") if r.status_code == 200 else ""
    demo_headers = {"Authorization": f"Bearer {demo_token}"}

    r = c.get("/v1/wallet", headers=demo_headers)
    suite.check("12 wallet read", r.status_code, 200, r.text)

    r = c.get("/v1/inventory", headers=suite.auth_headers())
    suite.check("13 inventory read", r.status_code, 200, r.text)

    demo_gold = c.get("/v1/wallet", headers=demo_headers).json().get("gold_balance", 0)
    purchasable = [
        p
        for p in products
        if (p.get("stock") is None or p.get("stock", 0) > 0)
        and p.get("price_gold", 0) <= demo_gold
    ]
    purchasable.sort(key=lambda p: p.get("price_gold", 0))
    product_id = purchasable[0]["id"] if purchasable else None
    if product_id is None:
        suite.check("14 npc purchase", 0, 200, "no products")
    else:
        r = c.post(
            "/v1/wallet/purchase",
            headers=demo_headers,
            json={
                "user_id": demo_player_id,
                "product_id": product_id,
                "currency": "gold",
                "idempotency_key": f"ga-e2e-{suffix}",
            },
        )
        suite.check("14 npc purchase", r.status_code, 200, r.text)

    r = c.get(
        "/v1/transactions",
        headers=demo_headers,
    )
    suite.check("15 transactions", r.status_code, 200, r.text)

    r = c.post(
        "/v1/wallet/transfer",
        headers=demo_headers,
        json={
            "from_user_id": demo_player_id,
            "to_user_id": suite.player_id,
            "currency": "gold",
            "amount": 1,
            "idempotency_key": f"ga-transfer-{suffix}",
        },
    )
    suite.check("16 wallet transfer", r.status_code, 200, r.text)

    r = c.post(
        "/v1/npc/npc_wang_boss_001/position",
        headers=suite.auth_headers(),
        json={"x": 20, "y": 30},
    )
    suite.check("17 npc position set", r.status_code, 200, r.text)

    r = c.get(
        "/v1/npc/npc_wang_boss_001/position",
        headers=suite.auth_headers(),
    )
    body = r.json() if r.status_code == 200 else {}
    suite.check(
        "18 npc position get",
        r.status_code,
        200,
        r.text,
        body.get("npc_id") == "npc_wang_boss_001",
    )

    r = c.post(
        "/v1/auth/login",
        json={"username": CREATOR_USER, "password": CREATOR_PASS},
    )
    creator_token = r.json().get("token", "") if r.status_code == 200 else ""
    creator_headers = {"Authorization": f"Bearer {creator_token}"}

    r = c.get("/v1/marketplace/revenue-summary", headers=creator_headers)
    suite.check("19 revenue summary", r.status_code, 200, r.text)

    r = c.get("/v1/marketplace/revenue", headers=creator_headers)
    suite.check("20 revenue list", r.status_code, 200, r.text)

    print(f"==> passed={suite.passed} failed={suite.failed}")
    return 0 if suite.failed == 0 else 1


if __name__ == "__main__":
    sys.exit(main())
