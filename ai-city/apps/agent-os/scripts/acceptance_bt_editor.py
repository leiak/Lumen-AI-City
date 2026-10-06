#!/usr/bin/env python3
"""acceptance_bt_editor — Phase C.4 7-step E2E for BT editor.

Verifies the bt-editor-api FastAPI service end-to-end against the
4 endpoints defined in apps/bt-editor-api/src/bt_editor_api/api/v1/bt.py:

  GET    /api/v1/bt/{npc_id}
  GET    /api/v1/bt/{npc_id}/{tree_name}
  POST   /api/v1/bt/{npc_id}/{tree_name}
  POST   /api/v1/bt/{npc_id}/{tree_name}/simulate

Steps:
  1. list trees for NPC             → 200 + JSON array
  2. get nonexistent tree           → 404
  3. save valid tree                → 200 + version=1
  4. save invalid (missing 'type')  → 400 + code=R_019
  5. save oversize (depth 12 > 10)  → 422 + code=R_019
  6. get after save                 → 200 + version=1
  7. simulate                       → 200 + trace log present

Exit code is 0 only on 7/7 PASS. Any FAIL → exit 1.

Usage:
  # Run against live bt-editor-api (default localhost:8090):
  python scripts/acceptance_bt_editor.py

  # Smoke mode — prints "SKIP" if API is unreachable (used in CI without
  # the full docker stack running):
  python scripts/acceptance_bt_editor.py --smoke
"""
from __future__ import annotations

import argparse
import json
import sys
import urllib.error
import urllib.request
from typing import Any, Callable

DEFAULT_API_BASE = "http://localhost:8090"
DEFAULT_NPC_ID = "npc_a_wang_boss_001"
DEFAULT_TREE_NAME = "greet_player"


def http(
    method: str,
    path: str,
    body: dict[str, Any] | None = None,
    base: str = DEFAULT_API_BASE,
    timeout: float = 10.0,
) -> tuple[int, dict[str, Any] | str]:
    """Issue an HTTP request, return (status, parsed body or text)."""
    url = f"{base}{path}"
    data = json.dumps(body).encode("utf-8") if body is not None else None
    req = urllib.request.Request(url, data=data, method=method)
    req.add_header("Content-Type", "application/json")
    try:
        with urllib.request.urlopen(req, timeout=timeout) as r:  # noqa: S310
            raw = r.read().decode("utf-8")
            try:
                return r.status, json.loads(raw)
            except json.JSONDecodeError:
                return r.status, raw
    except urllib.error.HTTPError as e:
        raw = e.read().decode("utf-8")
        try:
            return e.code, json.loads(raw)
        except json.JSONDecodeError:
            return e.code, raw
    except (urllib.error.URLError, ConnectionError, TimeoutError) as e:
        return 0, str(e)


# ---------------------------------------------------------------------------
# Step implementations
# ---------------------------------------------------------------------------


def step1_list(base: str, npc_id: str) -> tuple[bool, str]:
    status, body = http("GET", f"/api/v1/bt/{npc_id}", base=base)
    if status != 200:
        return False, f"expected 200, got {status}: {body}"
    if not isinstance(body, list):
        return False, f"expected list, got {type(body).__name__}: {body}"
    return True, f"{len(body)} trees"


def step2_get_missing(base: str, npc_id: str) -> tuple[bool, str]:
    status, _ = http("GET", f"/api/v1/bt/{npc_id}/nonexistent_tree_xyz", base=base)
    return (status == 404), f"got {status}"


def step3_save_valid(base: str, npc_id: str, tree_name: str) -> tuple[bool, str]:
    tree = {
        "version": "1.0.0",
        "root": {
            "id": "root",
            "type": "sequence",
            "children": [
                {
                    "id": "greet",
                    "type": "action",
                    "action": "say_to_player",
                    "args": {"text": "Hello!", "emotion": "happy"},
                }
            ],
        },
    }
    status, body = http(
        "POST",
        f"/api/v1/bt/{npc_id}/{tree_name}",
        body={"tree_json": tree},
        base=base,
    )
    if status != 200:
        return False, f"expected 200, got {status}: {body}"
    if not isinstance(body, dict) or body.get("version") != 1:
        return False, f"expected version=1, got {body}"
    return True, f"version={body.get('version')}"


def step4_save_invalid(base: str, npc_id: str) -> tuple[bool, str]:
    # Missing required 'type' discriminator — loader will reject.
    bad = {"version": "1.0.0", "root": {"id": "x"}}
    status, body = http(
        "POST",
        f"/api/v1/bt/{npc_id}/bad_tree",
        body={"tree_json": bad},
        base=base,
    )
    if status != 400:
        return False, f"expected 400, got {status}: {body}"
    code = (body or {}).get("detail", {}).get("code") if isinstance(body, dict) else None
    return code == "R_019", f"status={status} code={code}"


def step5_save_oversize(base: str, npc_id: str) -> tuple[bool, str]:
    # Build a depth-12 sequence (limit is 10). Each iteration wraps the
    # previous node in a new sequence, growing depth by 1.
    node: dict[str, Any] = {"id": "leaf", "type": "action", "action": "noop", "args": {}}
    for i in range(11):
        node = {"id": f"lvl_{i}", "type": "sequence", "children": [node]}
    tree = {"version": "1.0.0", "root": node}
    status, body = http(
        "POST",
        f"/api/v1/bt/{npc_id}/deep_tree",
        body={"tree_json": tree},
        base=base,
    )
    if status != 422:
        return False, f"expected 422, got {status}: {body}"
    code = (body or {}).get("detail", {}).get("code") if isinstance(body, dict) else None
    return code == "R_019", f"status={status} code={code}"


def step6_get_after_save(base: str, npc_id: str, tree_name: str) -> tuple[bool, str]:
    status, body = http("GET", f"/api/v1/bt/{npc_id}/{tree_name}", base=base)
    if status != 200:
        return False, f"expected 200, got {status}: {body}"
    if not isinstance(body, dict) or body.get("version") != 1:
        return False, f"expected version=1, got {body}"
    return True, f"version={body.get('version')}"


def step7_simulate(base: str, npc_id: str, tree_name: str) -> tuple[bool, str]:
    tree = {
        "version": "1.0.0",
        "root": {
            "id": "root",
            "type": "sequence",
            "children": [
                {
                    "id": "greet",
                    "type": "action",
                    "action": "say_to_player",
                    "args": {"text": "Hello!", "emotion": "happy"},
                }
            ],
        },
    }
    status, body = http(
        "POST",
        f"/api/v1/bt/{npc_id}/{tree_name}/simulate",
        body={
            "tree_json": tree,
            "state": {
                "player_position": [0, 0],
                "npc_state": {},
                "time_of_day": "noon",
                "max_ticks": 100,
            },
            "tick_limit": 10,
        },
        base=base,
    )
    if status != 200:
        return False, f"expected 200, got {status}: {body}"
    if not isinstance(body, dict):
        return False, f"expected dict body, got {type(body).__name__}"
    trace = body.get("trace")
    if not isinstance(trace, list) or len(trace) == 0:
        return False, f"expected non-empty trace list, got {trace}"
    final_status = body.get("status")
    return True, f"status={final_status} trace_len={len(trace)}"


STEPS: list[tuple[str, Callable[..., tuple[bool, str]]]] = [
    ("list trees",                    lambda b, n, t: step1_list(b, n)),
    ("get nonexistent → 404",         lambda b, n, t: step2_get_missing(b, n)),
    ("save valid tree",               lambda b, n, t: step3_save_valid(b, n, t)),
    ("save invalid → 400 R_019",      lambda b, n, t: step4_save_invalid(b, n)),
    ("save oversize → 422 R_019",     lambda b, n, t: step5_save_oversize(b, n)),
    ("get after save",                lambda b, n, t: step6_get_after_save(b, n, t)),
    ("simulate → trace log",          lambda b, n, t: step7_simulate(b, n, t)),
]


def main() -> int:
    parser = argparse.ArgumentParser(description="Phase C.4 bt-editor-api acceptance")
    parser.add_argument("--base", default=DEFAULT_API_BASE, help="bt-editor-api base URL")
    parser.add_argument("--npc-id", default=DEFAULT_NPC_ID, help="NPC id to test against")
    parser.add_argument("--tree-name", default=DEFAULT_TREE_NAME, help="Tree name for save/get")
    parser.add_argument(
        "--smoke",
        action="store_true",
        help="Smoke mode: SKIP (exit 0) if API is unreachable, FAIL otherwise",
    )
    args = parser.parse_args()

    print(f"=== acceptance_bt_editor (Phase C.4) ===")
    print(f"API base: {args.base}")
    print(f"NPC id:   {args.npc_id}")
    print(f"Tree:     {args.tree_name}")
    print(f"Smoke:    {args.smoke}")
    print()

    # Pre-flight: ensure the API is reachable (status 0 from urllib means
    # connection refused / DNS / timeout).
    pre_status, _ = http("GET", "/api/v1/bt/__healthcheck__", base=args.base)
    if pre_status == 0:
        if args.smoke:
            print("SKIP: bt-editor-api unreachable (smoke mode)")
            return 0
        print(f"FAIL: bt-editor-api unreachable at {args.base}")
        return 1

    results: list[bool] = []
    for i, (name, fn) in enumerate(STEPS, start=1):
        try:
            ok, msg = fn(args.base, args.npc_id, args.tree_name)
        except Exception as e:  # noqa: BLE001 — want to capture everything
            ok, msg = False, f"exception: {type(e).__name__}: {e}"
        results.append(ok)
        marker = "PASS" if ok else "FAIL"
        print(f"  Step {i}. {name:32s} [{marker}]  {msg}")

    passed = sum(results)
    total = len(results)
    print()
    if passed == total:
        print(f"=== {passed}/{total} PASS — BT editor ACCEPTED ===")
        return 0
    print(f"=== {passed}/{total} PASS — BT editor REJECTED ===")
    return 1


if __name__ == "__main__":
    sys.exit(main())
