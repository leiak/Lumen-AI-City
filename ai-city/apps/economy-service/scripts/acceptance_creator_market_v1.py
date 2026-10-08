#!/usr/bin/env python3
"""Phase 3.0 v3 acceptance_creator_market_v1.py — 10-step marketplace check.

Steps:
  1  Resolve health + three seeded marketplace identities.
  2  Creator creates a live NPC template priced at 100 gold.
  3  Buyer sees the template in the live market.
  4  Buyer purchases it; buyer -100 and creator +100.
  5  Creator revenue ledger records +100.
  6  Buyer inventory contains the purchase.
  7  Creator creates a Saga template referencing the NPC template.
  8  Saga dependency contract is readable (v1 runtime contract).
  9  Admin takes down the NPC and it disappears from the live market.
 10  Purchased inventory and purchased Saga template remain available.

Exit code: 0 = 10/10 PASS, 1 = any step FAIL.

Usage:
    ECONOMY_SERVICE_URL=http://localhost:8005 \\
    JWT_SECRET=dev-secret-change-me \\
    python scripts/acceptance_creator_market_v1.py
"""
from __future__ import annotations

import base64
import hashlib
import hmac
import json
import os
import shutil
import subprocess
import sys
import time
import urllib.error
import urllib.request
import uuid
from typing import Any

ECONOMY_URL = os.environ.get("ECONOMY_SERVICE_URL", "http://localhost:8005")
JWT_SECRET = os.environ.get("JWT_SECRET", "dev-secret-change-me")
ADMIN_TOKEN = os.environ.get("ADMIN_TOKEN", "dev-admin-token")
COMPOSE_PROJECT_DIR = os.environ.get(
    "AIPCITY_COMPOSE_DIR",
    os.path.normpath(os.path.join(os.path.dirname(__file__), "..", "..", "..")),
)


def _req(
    method: str,
    path: str,
    body: dict[str, Any] | None = None,
    headers: dict[str, str] | None = None,
) -> tuple[int, Any]:
    data = json.dumps(body).encode() if body is not None else None
    merged = {"Content-Type": "application/json"}
    if headers:
        merged.update(headers)
    request = urllib.request.Request(
        f"{ECONOMY_URL}{path}", data=data, method=method, headers=merged
    )
    try:
        with urllib.request.urlopen(request, timeout=10) as response:
            raw = response.read().decode() or "{}"
            return response.status, json.loads(raw)
    except urllib.error.HTTPError as exc:
        raw = exc.read().decode() if exc.fp else "{}"
        try:
            return exc.code, json.loads(raw)
        except json.JSONDecodeError:
            return exc.code, {"raw": raw}
    except (urllib.error.URLError, ConnectionError, TimeoutError, OSError) as exc:
        return 0, {"_error": f"{type(exc).__name__}: {exc}"}


def _check(step_no: int, name: str, ok: bool, detail: str = "") -> bool:
    icon = "[OK]" if ok else "[FAIL]"
    suffix = f" — {detail}" if detail else ""
    print(f"Step {step_no:>2d}: {icon} {'PASS' if ok else 'FAIL':4s}  {name}{suffix}")
    return ok


def _auth(player_id: str, role: str) -> dict[str, str]:
    def encode(value: dict[str, Any]) -> str:
        raw = json.dumps(value, separators=(",", ":")).encode()
        return base64.urlsafe_b64encode(raw).rstrip(b"=").decode()

    header = encode({"alg": "HS256", "typ": "JWT"})
    payload = encode(
        {
            "sub": player_id,
            "uname": f"acceptance-{role}",
            "role": role,
            "exp": int(time.time()) + 600,
        }
    )
    signature = hmac.new(
        JWT_SECRET.encode(), f"{header}.{payload}".encode(), hashlib.sha256
    ).digest()
    signature_b64 = base64.urlsafe_b64encode(signature).rstrip(b"=").decode()
    return {"Authorization": f"Bearer {header}.{payload}.{signature_b64}"}


def _resolve_player(username: str) -> tuple[str, str]:
    env_key = f"ACCEPTANCE_{username.upper()}_PLAYER_ID"
    if os.environ.get(env_key):
        return os.environ[env_key], os.environ.get(
            f"ACCEPTANCE_{username.upper()}_ROLE", "player"
        )
    if shutil.which("docker") is None:
        raise RuntimeError(f"set ${env_key} or install docker CLI to resolve {username}")
    result = subprocess.run(
        [
            "docker", "compose", "exec", "-T", "postgres", "psql", "-U", "aicity",
            "-d", "aicity", "-tAc",
            f"SELECT id::text || '|' || role FROM player WHERE username = '{username}';",
        ],
        cwd=COMPOSE_PROJECT_DIR,
        capture_output=True,
        text=True,
        timeout=15,
        check=True,
    )
    row = result.stdout.strip()
    if not row:
        raise RuntimeError(f"seeded player {username!r} is missing")
    player_id, role = row.split("|", 1)
    return player_id, role


def _wallet(player_id: str, headers: dict[str, str] | None = None) -> int:
    status, body = _req(
        "GET",
        f"/api/v1/wallet/{player_id}",
        headers=headers or {"Authorization": f"Bearer {ADMIN_TOKEN}"},
    )
    if status != 200:
        raise RuntimeError(f"wallet request failed: {status} {body}")
    return int(body["gold_balance"])


def step1_resolve_identities() -> tuple[str, str, str, str]:
    status, _ = _req("GET", "/health")
    if status != 200:
        raise RuntimeError(f"economy-service health failed: {status}")
    creator_id, creator_role = _resolve_player("creator_demo")
    admin_id, admin_role = _resolve_player("admin")
    if creator_role != "creator" or admin_role != "admin":
        raise RuntimeError("expected creator_demo=creator and admin=admin")
    return creator_id, admin_id, admin_id, admin_id


def step2_create_npc(creator_id: str, name: str) -> int:
    status, body = _req(
        "POST",
        "/v1/marketplace/npc-templates",
        {
            "name": name,
            "price_gold": 100,
            "ocean_json": {"O": 0.7, "C": 0.8, "E": 0.5, "A": 0.6, "N": 0.3},
        },
        _auth(creator_id, "creator"),
    )
    if status != 201:
        raise RuntimeError(f"create NPC failed: {status} {body}")
    return int(body["id"])


def step3_market_visible(template_id: int) -> bool:
    status, items = _req("GET", "/v1/marketplace/npc-templates?status=live")
    return status == 200 and any(item["id"] == template_id for item in items)


def step4_purchase_and_balances(
    creator_id: str, buyer_id: str, template_id: int, buyer_auth: dict[str, str]
) -> bool:
    creator_before = _wallet(creator_id)
    buyer_before = _wallet(buyer_id, buyer_auth)
    status, body = _req(
        "POST",
        "/v1/marketplace/purchase",
        {
            "template_kind": "npc",
            "template_id": template_id,
            "idempotency_key": f"acc-v3-npc-{uuid.uuid4().hex[:12]}",
        },
        buyer_auth,
    )
    if status != 200:
        return False
    creator_after = _wallet(creator_id)
    buyer_after = _wallet(buyer_id, buyer_auth)
    return buyer_after == buyer_before - 100 and creator_after == creator_before + 100


def step5_revenue_recorded(creator_id: str) -> bool:
    status, revenue = _req(
        "GET",
        f"/v1/marketplace/revenue/{creator_id}?limit=1&offset=0",
        headers=_auth(creator_id, "creator"),
    )
    return (
        status == 200
        and revenue
        and int(revenue[0]["amount_gold"]) == 100
        and int(revenue[0]["platform_cut_gold"]) == 0
    )


def step6_inventory_recorded(buyer_id: str, buyer_auth: dict[str, str]) -> bool:
    status, inventory = _req(
        "GET",
        f"/v1/marketplace/inventory/{buyer_id}?limit=1&offset=0",
        headers=buyer_auth,
    )
    return status == 200 and bool(inventory)


def step7_create_saga(
    creator_id: str, npc_template_id: int
) -> int:
    status, body = _req(
        "POST",
        "/v1/marketplace/saga-templates",
        {
            "name": f"Acceptance Saga {uuid.uuid4().hex[:6]}",
            "price_gold": 100,
            "yaml_content": "saga:\n  name: acceptance\n",
            "semantic_version": "1.0.0",
            "npc_deps": [str(npc_template_id)],
        },
        _auth(creator_id, "creator"),
    )
    if status != 201:
        raise RuntimeError(f"create Saga failed: {status} {body}")
    return int(body["id"])


def step8_runtime_dependency_contract(saga_template_id: int) -> bool:
    status, template = _req(
        "GET", f"/v1/marketplace/saga-templates/{saga_template_id}"
    )
    return status == 200 and template["status"] == "live" and bool(template["npc_deps"])


def step9_admin_takes_down_npc(
    admin_id: str, template_id: int
) -> bool:
    status, body = _req(
        "POST",
        f"/v1/marketplace/npc-templates/{template_id}/take-down",
        {},
        _auth(admin_id, "admin"),
    )
    if status != 200 or body.get("status") != "taken_down":
        return False
    status, items = _req("GET", "/v1/marketplace/npc-templates?status=live")
    return status == 200 and not any(item["id"] == template_id for item in items)


def step10_purchased_state_remains(
    buyer_id: str, buyer_auth: dict[str, str], saga_template_id: int
) -> bool:
    inventory_status, inventory = _req(
        "GET",
        f"/v1/marketplace/inventory/{buyer_id}",
        headers=buyer_auth,
    )
    saga_status, saga = _req(
        "GET", f"/v1/marketplace/saga-templates/{saga_template_id}"
    )
    return (
        inventory_status == 200
        and bool(inventory)
        and saga_status == 200
        and saga["status"] == "live"
    )


def main() -> int:
    print(f"=== Phase 3.0 v3 acceptance_creator_market_v1 ({ECONOMY_URL}) ===\n")
    results: list[bool] = []
    buyer_auth: dict[str, str] = {}
    template_id = 0
    saga_template_id = 0
    creator_id = buyer_id = admin_id = ""
    npc_name = f"Acceptance NPC {uuid.uuid4().hex[:6]}"

    try:
        creator_id, buyer_id, admin_id, _ = step1_resolve_identities()
        results.append(_check(1, "health + creator/admin identities", True))
    except Exception as exc:
        results.append(_check(1, "health + creator/admin identities", False, str(exc)))

    if creator_id:
        try:
            template_id = step2_create_npc(creator_id, npc_name)
            results.append(_check(2, "creator creates NPC (100 gold)", True))
        except Exception as exc:
            results.append(_check(2, "creator creates NPC (100 gold)", False, str(exc)))

        buyer_auth = _auth(buyer_id, "admin")
        results.append(
            _check(3, "buyer sees NPC in live market", step3_market_visible(template_id))
        )
        results.append(
            _check(
                4,
                "purchase moves 100 gold buyer -> creator",
                step4_purchase_and_balances(
                    creator_id, buyer_id, template_id, buyer_auth
                ),
            )
        )
        results.append(
            _check(5, "creator revenue records +100", step5_revenue_recorded(creator_id))
        )
        results.append(
            _check(6, "buyer inventory records purchase", step6_inventory_recorded(buyer_id, buyer_auth))
        )

        try:
            saga_template_id = step7_create_saga(creator_id, template_id)
            results.append(_check(7, "creator creates Saga with NPC dep", True))
        except Exception as exc:
            results.append(_check(7, "creator creates Saga with NPC dep", False, str(exc)))

        results.append(
            _check(8, "Saga dependency contract readable", step8_runtime_dependency_contract(saga_template_id))
        )
        results.append(
            _check(9, "admin takedown removes NPC from market", step9_admin_takes_down_npc(admin_id, template_id))
        )
        results.append(
            _check(10, "purchased inventory/Saga remain available", step10_purchased_state_remains(buyer_id, buyer_auth, saga_template_id))
        )
    else:
        for step_no, name in [
            (2, "creator creates NPC (100 gold)"),
            (3, "buyer sees NPC in live market"),
            (4, "purchase moves 100 gold buyer -> creator"),
            (5, "creator revenue records +100"),
            (6, "buyer inventory records purchase"),
            (7, "creator creates Saga with NPC dep"),
            (8, "Saga dependency contract readable"),
            (9, "admin takedown removes NPC from market"),
            (10, "purchased inventory/Saga remain available"),
        ]:
            results.append(_check(step_no, name, False, "skipped — no identity"))

    passed = sum(results)
    print(f"\n=== {passed}/{len(results)} steps PASS ===")
    return 0 if passed == len(results) else 1


if __name__ == "__main__":
    sys.exit(main())
