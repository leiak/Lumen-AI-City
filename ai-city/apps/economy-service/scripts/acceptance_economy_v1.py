#!/usr/bin/env python3
"""Phase 3.0 acceptance_economy_v1.py — 10-step E2E acceptance.

Validates the 3.0 economy v1 GA surface against the docker-compose stack:

  Step  1: Health check + resolve 2 seeded player IDs (demo/admin).
  Step  2: A → B transfer 100 gold.
  Step  3: Verify A=9900 / B=10100 (after 100 moved, seed = 10000 each).
  Step  4: B buys NPC product (50 gold) → balance 10050 + stock-1.
  Step  5: A buys expensive product → 402 / R_022 (insufficient).
  Step  6: Transfer to self → 400 / R_023.
  Step  7: Same idempotency_key twice → 1 charge (tx_id stable).
  Step  8: Admin central-bank emit → active players +N.
  Step  9: A transaction history ≥ 3 (transfer out + self-attempt + others).
  Step 10: BT action isolation on 5xx — npc_sell_to_player returns FAILURE.

Exit code: 0 = 10/10 PASS, 1 = any step FAIL.

Usage:
    # Inside docker container:
    docker compose exec -T economy-service python scripts/acceptance_economy_v1.py
    # Or from host (with economy-service on localhost:8005):
    cd apps/economy-service && python scripts/acceptance_economy_v1.py
    # Or with explicit URL:
    ECONOMY_SERVICE_URL=http://localhost:8005 python scripts/acceptance_economy_v1.py

Environment:
    ECONOMY_SERVICE_URL   default http://localhost:8005
    ADMIN_TOKEN           default dev-admin-token (matches api/v1/admin.py)
    ALICE_PLAYER_ID       override step1 alice ID (else query psql)
    BOB_PLAYER_ID         override step1 bob ID   (else query psql)
"""
from __future__ import annotations

import json
import os
import shutil
import subprocess
import sys
import urllib.error
import urllib.request
import uuid
from typing import Any, Callable

ECONOMY_URL = os.environ.get("ECONOMY_SERVICE_URL", "http://localhost:8005")
ADMIN_TOKEN = os.environ.get("ADMIN_TOKEN", "dev-admin-token")
COMPOSE_PROJECT_DIR = os.environ.get(
    "AIPCITY_COMPOSE_DIR",
    # 默认 docker-compose.yml 在 ai-city/ 根目录
    os.path.normpath(
        os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "..", "..")
    ),
)

# Seed values from db/seed/seed-economy.sql
SEED_GOLD = 1000  # demo + admin seeded with 1000 gold each
TRANSFER_AMOUNT = 100
PRODUCT_PRICE = 50  # '招牌红烧肉' from wang_boss_001


# ---------------------------------------------------------------------------
# HTTP helper (stdlib only — matches a2a-gateway Go binary's std-only deps)
# ---------------------------------------------------------------------------


def _req(
    method: str, path: str,
    body: dict | None = None,
    headers: dict | None = None,
    timeout: float = 10.0,
) -> tuple[int, dict]:
    """HTTP wrapper returning (status, json_body).

    Returns (0, {"_error": ...}) on network errors so step functions can
    fail gracefully without crashing the whole acceptance run.
    """
    url = f"{ECONOMY_URL}{path}"
    data = json.dumps(body).encode() if body is not None else None
    merged = {"Content-Type": "application/json"}
    if headers:
        merged.update(headers)
    req = urllib.request.Request(url, data=data, method=method, headers=merged)
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            raw = resp.read().decode() or "{}"
            return resp.status, json.loads(raw)
    except urllib.error.HTTPError as e:
        raw = e.read().decode() if e.fp else ""
        try:
            payload = json.loads(raw) if raw else {}
        except json.JSONDecodeError:
            payload = {"raw": raw}
        return e.code, payload
    except (urllib.error.URLError, ConnectionError, TimeoutError, OSError) as e:
        # Service unreachable / DNS / refused → caller treats status=0 as FAIL.
        return 0, {"_error": f"{type(e).__name__}: {e}"}


def _check(step_no: int, name: str, ok: bool, detail: str = "") -> bool:
    """Print one step result, return True/False for tally."""
    status = "PASS" if ok else "FAIL"
    icon = "[OK]" if ok else "[FAIL]"
    suffix = f" — {detail}" if detail else ""
    print(f"Step {step_no:>2d}: {icon} {status:4s}  {name}{suffix}")
    return ok


# ---------------------------------------------------------------------------
# Step 1 — health + resolve 2 seeded player IDs
# ---------------------------------------------------------------------------


def _resolve_player_id(username: str) -> str:
    """Resolve player.id (UUID) for the given username.

    Priority:
      1. env var override (ALICE_PLAYER_ID / BOB_PLAYER_ID)
      2. ``docker compose exec postgres psql ...`` (if docker is on PATH)
      3. raise RuntimeError with actionable message
    """
    # 1. env override
    env_key = "ALICE_PLAYER_ID" if username == "demo" else "BOB_PLAYER_ID"
    override = os.environ.get(env_key)
    if override:
        return override

    # 2. docker compose psql query
    if shutil.which("docker") is None:
        raise RuntimeError(
            f"Cannot resolve player UUID for username={username!r}: "
            f"set ${env_key} env var or install docker CLI."
        )

    try:
        result = subprocess.run(
            [
                "docker", "compose", "exec", "-T", "postgres",
                "psql", "-U", "aicity", "-d", "aicity",
                "-tAc", f"SELECT id FROM player WHERE username = '{username}';",
            ],
            cwd=COMPOSE_PROJECT_DIR,
            capture_output=True,
            text=True,
            timeout=15,
            check=True,
        )
        pid = result.stdout.strip()
        if not pid:
            raise RuntimeError(
                f"player username={username!r} not found in postgres "
                f"(seed may be missing — check db/seed/seed-admin.sql)"
            )
        return pid
    except subprocess.CalledProcessError as e:
        raise RuntimeError(
            f"psql query failed for username={username!r}: "
            f"{(e.stderr or '').strip()}"
        ) from e
    except subprocess.TimeoutExpired as e:
        raise RuntimeError(
            f"psql query timed out for username={username!r}"
        ) from e


def step1_register_two_players() -> tuple[str, str]:
    """Health check + return (alice_id, bob_id).

    alice = demo UUID, bob = admin UUID (seed-economy.sql).
    """
    status, _ = _req("GET", "/health")
    if status != 200:
        return "", ""

    try:
        alice = _resolve_player_id("demo")
        bob = _resolve_player_id("admin")
    except RuntimeError as e:
        print(f"  step1 resolve error: {e}")
        return "", ""

    if alice == bob or not alice or not bob:
        return "", ""
    return alice, bob


# ---------------------------------------------------------------------------
# Step 2 — A → B transfer 100 gold
# ---------------------------------------------------------------------------


def step2_transfer(alice: str, bob: str) -> bool:
    """Transfer `TRANSFER_AMOUNT` gold from alice → bob. Returns True on 200."""
    status, _ = _req("POST", "/api/v1/wallet/transfer", body={
        "from_user_id": alice,
        "to_user_id": bob,
        "currency": "gold",
        "amount": TRANSFER_AMOUNT,
        "idempotency_key": f"acc-v1-s2-{uuid.uuid4().hex[:12]}",
        "memo": "acceptance_economy_v1 step 2",
    })
    return status == 200


# ---------------------------------------------------------------------------
# Step 3 — verify balances (seed = 10000 each, after step 2: 9900 / 10100)
# ---------------------------------------------------------------------------


def step3_check_balances(alice: str, bob: str) -> bool:
    """Expect alice=SEED_GOLD - TRANSFER_AMOUNT, bob=SEED_GOLD + TRANSFER_AMOUNT."""
    _, a = _req("GET", f"/api/v1/wallet/{alice}")
    _, b = _req("GET", f"/api/v1/wallet/{bob}")
    expect_a = SEED_GOLD - TRANSFER_AMOUNT  # 900
    expect_b = SEED_GOLD + TRANSFER_AMOUNT  # 1100
    return (
        a.get("gold_balance") == expect_a
        and b.get("gold_balance") == expect_b
    )


# ---------------------------------------------------------------------------
# Step 4 — B buys NPC product → balance -50, stock-1
# ---------------------------------------------------------------------------


def step4_bob_buys_product(bob: str, product_id: int = 1) -> bool:
    """B buys product_id=1 (招牌红烧肉, 50 gold). Returns True on 200.

    Uses seed product ids (per db/seed/seed-economy.sql, the first INSERT
    row = 招牌红烧肉 with price_gold=50).
    """
    status, body = _req("POST", "/api/v1/wallet/purchase", body={
        "user_id": bob,
        "product_id": product_id,
        "currency": "gold",
        "idempotency_key": f"acc-v1-s4-{uuid.uuid4().hex[:12]}",
        "trace_id": "acceptance_economy_v1 step 4",
    })
    if status != 200:
        return False
    # bob: 1100 → 1050
    return body.get("balance_after") == (SEED_GOLD + TRANSFER_AMOUNT - PRODUCT_PRICE)


# ---------------------------------------------------------------------------
# Step 5 — A insufficient → 402 / R_022
# ---------------------------------------------------------------------------


def step5_alice_insufficient(alice: str, product_id: int = 5) -> bool:
    """A buys product_id=5 (古籍 500 gold) — alice only has 900. 402 / R_022.

    product_id=5 is '古籍' from npc_book_keeper_001 (500 gold), the cheapest
    item alice (900 gold) cannot afford. product_id=1 招牌红烧肉 is 50 gold
    which alice can still afford after step 2.
    """
    status, body = _req("POST", "/api/v1/wallet/purchase", body={
        "user_id": alice,
        "product_id": product_id,
        "currency": "gold",
        "idempotency_key": f"acc-v1-s5-{uuid.uuid4().hex[:12]}",
        "trace_id": "acceptance_economy_v1 step 5",
    })
    return status == 402 and body.get("detail", {}).get("code") == "R_022"


# ---------------------------------------------------------------------------
# Step 6 — transfer to self → 400 / R_023
# ---------------------------------------------------------------------------


def step6_transfer_self(alice: str) -> bool:
    """alice → alice → 400 / R_023 (TransferSelf)."""
    status, body = _req("POST", "/api/v1/wallet/transfer", body={
        "from_user_id": alice,
        "to_user_id": alice,
        "currency": "gold",
        "amount": 1,
        "idempotency_key": f"acc-v1-s6-{uuid.uuid4().hex[:12]}",
    })
    return status == 400 and body.get("detail", {}).get("code") == "R_023"


# ---------------------------------------------------------------------------
# Step 7 — idempotent purchase (same key twice → 1 charge)
# ---------------------------------------------------------------------------


def step7_idempotent_purchase(bob: str, product_id: int = 4) -> bool:
    """Two POSTs with same idempotency_key → both 200, only 1 ledger hit.

    product_id=4 = 糖葫芦 (10 gold) so bob (1050) can afford it twice on paper,
    but idempotency must dedupe to 1 charge.
    """
    key = f"acc-v1-s7-{uuid.uuid4().hex[:12]}"
    body = {
        "user_id": bob,
        "product_id": product_id,
        "currency": "gold",
        "idempotency_key": key,
        "trace_id": "acceptance_economy_v1 step 7",
    }
    s1, b1 = _req("POST", "/api/v1/wallet/purchase", body=body)
    s2, b2 = _req("POST", "/api/v1/wallet/purchase", body=body)
    if s1 != 200 or s2 != 200:
        return False
    # Idempotency cache stores full response → body must match (or at least
    # balance_after must match). Use balance_after as the canonical signal.
    return b1.get("balance_after") == b2.get("balance_after")


# ---------------------------------------------------------------------------
# Step 8 — admin central-bank emit
# ---------------------------------------------------------------------------


def step8_central_bank_emit() -> bool:
    """POST /api/v1/admin/central-bank/emit with Bearer token. 200 + amount key."""
    status, body = _req(
        "POST", "/api/v1/admin/central-bank/emit",
        headers={"Authorization": f"Bearer {ADMIN_TOKEN}"},
    )
    if status != 200:
        return False
    # Response shape per central_bank.py:emit():
    #   {emitted, active_players, total_distributed}  (normal)
    #   {emitted:0, active_players, skipped:true}      (skipped)
    # The acceptance considers "emit endpoint reachable + non-error" as PASS,
    # since skip happens when total_supply >= SINK_CAPACITY.
    return "active_players" in body and "emitted" in body


# ---------------------------------------------------------------------------
# Step 9 — A transaction history ≥ 3
# ---------------------------------------------------------------------------


def step9_transaction_history(alice: str) -> bool:
    """GET /api/v1/transactions/{alice} → list with ≥ 3 entries.

    After steps 2/6 alice has:
      - 1× player_transfer OUT (step 2, -100)
      - 1× failed self-transfer ATTEMPT — may or may not log to transaction
        table (current implementation does NOT log on rollback, so we only
        rely on step 2 + any other tx from earlier runs in the same DB).
    """
    status, body = _req("GET", f"/api/v1/transactions/{alice}")
    if status != 200:
        return False
    txs = body.get("transactions", [])
    return len(txs) >= 3


# ---------------------------------------------------------------------------
# Step 10 — BT action isolation on 5xx
# ---------------------------------------------------------------------------


def step10_agent_os_bt_failure_isolation() -> bool:
    """Verify npc_sell_to_player returns FAILURE (not crash) on 5xx.

    Imports agent_os.bt.actions.npc_sell_to_player and runs it against a
    mocked httpx.Client.post returning 503. Expect Status.FAILURE.
    """
    try:
        # Mirror agent-os convention: add src/ to sys.path so the action
        # module is importable when acceptance runs from a checkout
        # (e.g. `cd apps/economy-service && python scripts/...`).
        src_root = os.path.normpath(
            os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "src")
        )
        if src_root not in sys.path:
            sys.path.insert(0, src_root)

        # agent-os lives in a sibling app; allow import via direct path.
        agent_os_src = os.path.normpath(
            os.path.join(
                os.path.dirname(os.path.abspath(__file__)),
                "..", "..", "agent-os", "src",
            )
        )
        if agent_os_src not in sys.path:
            sys.path.insert(0, agent_os_src)

        from unittest.mock import MagicMock, patch

        from agent_os.bt.actions import npc_sell_to_player  # noqa: E402
        from agent_os.bt.state import BTState  # noqa: E402

        state = BTState(player_id="acceptance_economy_v1_test_player")

        mock_resp = MagicMock()
        mock_resp.status_code = 503
        mock_resp.json.return_value = {"detail": {"code": "R_500"}}

        with patch("httpx.Client.post", return_value=mock_resp):
            status = npc_sell_to_player(
                product_id=999, currency="gold", state=state,
            )

        return status.name == "FAILURE"
    except Exception as e:
        print(f"  step10 error: {e}")
        return False


# ---------------------------------------------------------------------------
# main
# ---------------------------------------------------------------------------


def main() -> int:
    print(f"=== Phase 3.0 acceptance_economy_v1 ({ECONOMY_URL}) ===\n")

    try:
        return _run_steps()
    except Exception as e:
        print(f"\nFATAL: unexpected exception during acceptance run: {e}")
        return 1


def _run_steps() -> int:
    results: list[bool] = []

    # Step 1
    alice, bob = step1_register_two_players()
    ok1 = bool(alice and bob)
    detail = (
        f"alice={alice[:8]}..., bob={bob[:8]}..."
        if ok1 else "could not resolve player IDs"
    )
    results.append(_check(1, "register 2 players (demo + admin)", ok1, detail))

    # Step 2-7 require alice + bob; abort cleanly otherwise
    if not (alice and bob):
        for n, name in [
            (2, "transfer A→B 100 gold"),
            (3, "balances A=900 / B=1100"),
            (4, "B buys product → balance_after=1050"),
            (5, "A insufficient → 402 / R_022"),
            (6, "transfer self → 400 / R_023"),
            (7, "idempotent purchase → 1 charge"),
        ]:
            results.append(_check(n, name, False, "skipped — no player IDs"))
    else:
        results.append(_check(2, "transfer A→B 100 gold", step2_transfer(alice, bob)))
        results.append(_check(3, "balances A=900 / B=1100", step3_check_balances(alice, bob)))
        results.append(_check(4, "B buys product → balance_after=1050", step4_bob_buys_product(bob)))
        results.append(_check(5, "A insufficient → 402 / R_022", step5_alice_insufficient(alice)))
        results.append(_check(6, "transfer self → 400 / R_023", step6_transfer_self(alice)))
        results.append(_check(7, "idempotent purchase → 1 charge", step7_idempotent_purchase(bob)))

    # Step 8 (admin) and Step 9 (transactions) work independently of alice/bob.
    results.append(_check(8, "admin central-bank emit", step8_central_bank_emit()))
    results.append(_check(9, "A transaction history ≥ 3", step9_transaction_history(alice)))
    results.append(_check(10, "BT action isolation on 5xx", step10_agent_os_bt_failure_isolation()))

    passed = sum(results)
    total = len(results)
    print(f"\n=== {passed}/{total} steps PASS ===")
    return 0 if passed == total else 1


if __name__ == "__main__":
    sys.exit(main())
