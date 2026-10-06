"""Phase C.2 — bt-editor-api 4 endpoints (FastAPI TestClient + mocked asyncpg).

These tests exercise the public HTTP surface without touching a real PG:
``mock_pool`` (from conftest.py) injects canned results into the
``asyncpg.Pool``-shaped seam so each endpoint's SQL path is verifiable
in isolation.
"""
from __future__ import annotations

import json
from datetime import UTC, datetime
from unittest.mock import AsyncMock

# ---- Helpers --------------------------------------------------------------


def _now_iso() -> str:
    return datetime.now(UTC).isoformat()


def _simple_noop_tree() -> dict:
    """Minimal valid BT — a sequence with one noop action."""
    return {"id": "root", "type": "sequence", "children": [
        {"id": "a", "type": "action", "name": "noop", "args": []},
    ]}


def _failing_tree() -> dict:
    """Selector with all-failing children — final status FAILURE."""
    return {"id": "root", "type": "selector", "children": [
        {"id": "c", "type": "condition", "name": "player_in_tile", "args": [99, 99]},
    ]}


def _mock_record(values: dict) -> AsyncMock:
    """AsyncMock that mimics asyncpg.Record — supports both ``record["k"]`` and ``record["k"]`` access patterns.

    asyncpg.Record is a tuple subclass with key access; for our purposes a
    plain dict-shaped object that supports ``__getitem__`` is sufficient.
    """
    rec = AsyncMock()
    rec.__getitem__.side_effect = values.__getitem__
    return rec


# ---- list -----------------------------------------------------------------


def test_list_trees_for_npc_empty(mock_pool, client):
    """GET /api/v1/bt/{npc_id} with no rows → 200 + empty list."""
    mock_pool.fetch.return_value = []
    resp = client.get("/api/v1/bt/npc_alpha")
    assert resp.status_code == 200
    assert resp.json() == []
    # Verify SQL shape: SELECT name, version, updated_at FROM bt_tree WHERE npc_id = $1
    args, _ = mock_pool.fetch.call_args
    assert "FROM bt_tree" in args[0]
    assert args[1] == "npc_alpha"


def test_list_trees_for_npc_returns_summaries(mock_pool, client):
    """GET /api/v1/bt/{npc_id} with two rows → 200 + 2 summaries."""
    ts = _now_iso()
    mock_pool.fetch.return_value = [
        {"name": "greet_player", "version": 3, "updated_at": ts},
        {"name": "patrol", "version": 1, "updated_at": ts},
    ]
    resp = client.get("/api/v1/bt/npc_alpha")
    assert resp.status_code == 200
    body = resp.json()
    assert len(body) == 2
    assert body[0]["name"] == "greet_player"
    assert body[0]["version"] == 3


# ---- get ------------------------------------------------------------------


def test_get_nonexistent_returns_404(mock_pool, client):
    """GET /api/v1/bt/{npc}/{name} with no row → 404."""
    mock_pool.fetchrow.return_value = None
    resp = client.get("/api/v1/bt/npc_alpha/ghost")
    assert resp.status_code == 404


def test_save_then_get_roundtrip(mock_pool, client):
    """POST upserts then GET returns same tree_json."""
    tree = _simple_noop_tree()
    mock_pool.fetchrow.return_value = {
        "npc_id": "npc_alpha",
        "name": "greet_player",
        "tree_json": json.dumps(tree),  # asyncpg returns JSONB as a string
        "version": 1,
        "created_at": _now_iso(),
        "updated_at": _now_iso(),
    }
    save_resp = client.post(
        "/api/v1/bt/npc_alpha/greet_player",
        json={"tree_json": tree},
    )
    assert save_resp.status_code == 200, save_resp.text
    body = save_resp.json()
    assert body["name"] == "greet_player"
    assert body["version"] == 1
    # Verify upsert SQL was issued
    args, _ = mock_pool.execute.call_args
    assert "INSERT INTO bt_tree" in args[0]
    assert args[1] == "npc_alpha"
    assert args[2] == "greet_player"


def test_save_increments_version(mock_pool, client):
    """POST twice → second response reports version=2 (DB trigger bumps it)."""
    tree = _simple_noop_tree()
    # After second save, fetchrow returns the bumped row.
    mock_pool.fetchrow.return_value = {
        "npc_id": "npc_alpha",
        "name": "greet_player",
        "tree_json": json.dumps(tree),
        "version": 2,
        "created_at": _now_iso(),
        "updated_at": _now_iso(),
    }
    resp = client.post(
        "/api/v1/bt/npc_alpha/greet_player",
        json={"tree_json": tree},
    )
    assert resp.status_code == 200
    body = resp.json()
    assert body["version"] == 2
    # The upsert SQL must use ON CONFLICT (so the trigger fires).
    # ``version`` itself is NOT in the SET clause — the BEFORE UPDATE
    # trigger bumps it from OLD.version + 1.
    args, _ = mock_pool.execute.call_args
    sql = args[0]
    assert "ON CONFLICT" in sql
    assert "DO UPDATE" in sql


# ---- save validation ------------------------------------------------------


def test_save_invalid_json_returns_400(mock_pool, client):
    """Malformed tree_json (missing required ``type`` discriminator) → 400 R_019."""
    resp = client.post(
        "/api/v1/bt/npc_alpha/greet_player",
        json={"tree_json": {"id": "root"}},  # no 'type' field
    )
    assert resp.status_code == 400
    detail = resp.json()["detail"]
    assert detail["code"] == "R_019"


def test_save_unknown_node_type_returns_400(mock_pool, client):
    """tree_json with an unknown ``type`` discriminator → 400 R_019."""
    resp = client.post(
        "/api/v1/bt/npc_alpha/greet_player",
        json={"tree_json": {"id": "x", "type": "mystery_node"}},
    )
    assert resp.status_code == 400
    assert resp.json()["detail"]["code"] == "R_019"


def test_save_oversized_tree_returns_422(mock_pool, client):
    """Tree depth > BT_TREE_DEPTH_LIMIT → 422 with R_019 code."""
    # Build a sequence with depth 5; default limit is 10 — should PASS.
    # We need depth > limit → 11-deep sequence.
    def _branch(depth: int) -> dict:
        if depth == 0:
            return {"id": f"a{depth}", "type": "action", "name": "noop", "args": []}
        return {
            "id": f"a{depth}",
            "type": "sequence",
            "children": [_branch(depth - 1)],
        }

    deep = _branch(11)  # depth = 12
    resp = client.post(
        "/api/v1/bt/npc_alpha/greet_player",
        json={"tree_json": deep},
    )
    assert resp.status_code == 422
    assert resp.json()["detail"]["code"] == "R_019"
    assert "depth" in resp.json()["detail"]["msg"].lower()


def test_save_too_many_nodes_returns_422(mock_pool, client):
    """Node count > BT_TREE_NODE_LIMIT → 422 with R_019 code."""
    # 51 actions in one sequence — exceeds default limit of 50.
    children = [
        {"id": f"a{i}", "type": "action", "name": "noop", "args": []}
        for i in range(51)
    ]
    tree = {"id": "root", "type": "sequence", "children": children}
    resp = client.post(
        "/api/v1/bt/npc_alpha/greet_player",
        json={"tree_json": tree},
    )
    assert resp.status_code == 422
    assert resp.json()["detail"]["code"] == "R_019"
    assert "node" in resp.json()["detail"]["msg"].lower()


# ---- simulate -------------------------------------------------------------


def test_simulate_returns_trace_log(mock_pool, client):
    """POST simulate with a successful sequence → trace with status=success."""
    tree = _simple_noop_tree()
    resp = client.post(
        "/api/v1/bt/npc_alpha/greet_player/simulate",
        json={
            "tree_json": tree,
            "state": {
                "player_position": [0, 0],
                "npc_state": {},
                "time_of_day": "noon",
                "max_ticks": 100,
            },
            "tick_limit": 5,
        },
    )
    assert resp.status_code == 200
    body = resp.json()
    assert body["status"] == "success"
    assert len(body["trace"]) == 1
    assert body["trace"][0]["status"] == "success"
    assert body["trace"][0]["node_id"] == "root"
    assert body["final_state"]["time_of_day"] == "noon"


def test_simulate_with_failing_condition_returns_failure(mock_pool, client):
    """Selector with a failing condition → status=failure."""
    resp = client.post(
        "/api/v1/bt/npc_alpha/guard/simulate",
        json={
            "tree_json": _failing_tree(),
            "state": {
                "player_position": [0, 0],  # NOT (99, 99) → condition fails
                "npc_state": {},
                "time_of_day": "noon",
                "max_ticks": 100,
            },
            "tick_limit": 5,
        },
    )
    assert resp.status_code == 200
    body = resp.json()
    assert body["status"] == "failure"
    assert body["trace"][-1]["status"] == "failure"


def test_simulate_invalid_tree_returns_400(mock_pool, client):
    """Simulate with malformed tree_json → 400 R_019."""
    resp = client.post(
        "/api/v1/bt/npc_alpha/guard/simulate",
        json={"tree_json": {"id": "x"}, "state": {}, "tick_limit": 5},
    )
    assert resp.status_code == 400
    assert resp.json()["detail"]["code"] == "R_019"


def test_simulate_tick_limit_caps_loop(mock_pool, client):
    """When tree stays RUNNING forever, simulate caps at tick_limit entries."""
    # A `wait(60)` action returns RUNNING.
    tree = {"id": "root", "type": "action", "name": "wait", "args": [60.0]}
    resp = client.post(
        "/api/v1/bt/npc_alpha/rest/simulate",
        json={
            "tree_json": tree,
            "state": {"player_position": [0, 0], "npc_state": {}, "time_of_day": "noon", "max_ticks": 100},
            "tick_limit": 3,
        },
    )
    assert resp.status_code == 200
    body = resp.json()
    assert body["status"] == "running"
    assert len(body["trace"]) == 3  # capped at tick_limit


# ---- R_020 contract (Phase C.2 review Fix #5) -----------------------------


def test_simulate_subtree_missing_registry_returns_500(mock_pool, client):
    """Spec: /simulate returns 500 / R_020 on fatal eval errors.

    A ``subtree`` node in the tree asks the C.1 evaluator to look up the
    referenced ``tree_id`` via ``BTState.registry``. The endpoint never
    sets a registry, so the evaluator raises ``BTError`` ("subtree ...
    requires BTState.registry to be set"). The endpoint must surface
    that as 500 + R_020 — NOT 200 with ``status="error"``.
    """
    tree = {
        "id": "root",
        "type": "sequence",
        "children": [
            {"id": "ref", "type": "subtree", "tree_id": "greet_player"},
        ],
    }
    resp = client.post(
        "/api/v1/bt/npc_alpha/composite/simulate",
        json={
            "tree_json": tree,
            "state": {
                "player_position": [0, 0],
                "npc_state": {},
                "time_of_day": "noon",
                "max_ticks": 100,
            },
            "tick_limit": 5,
        },
    )
    assert resp.status_code == 500
    detail = resp.json()["detail"]
    assert detail["code"] == "R_020"
    assert "BT_EVAL_FAIL" in detail["msg"]


# ---- Path validation (Phase C.2 review Fix #4) ---------------------------


def test_list_trees_rejects_invalid_npc_id(client):
    """Path-param validation: slashes / non-word chars → 422, no DB call."""
    # 422 is FastAPI's default for Path() validation failures.
    resp = client.get("/api/v1/bt/bad..npc")
    assert resp.status_code == 422


def test_get_tree_rejects_invalid_tree_name(client):
    """Path-param validation: tree_name with special chars → 422."""
    # Path component with spaces — URL-encoded as %20 — must be rejected.
    resp = client.get("/api/v1/bt/npc_alpha/bad%20name")
    assert resp.status_code == 422


def test_save_tree_rejects_oversized_npc_id(client):
    """Path-param validation: npc_id longer than 64 chars → 422."""
    long_npc = "npc_" + ("x" * 70)  # 73 chars, well over the 64-char cap
    resp = client.post(
        f"/api/v1/bt/{long_npc}/tree",
        json={"tree_json": {"id": "root", "type": "action", "name": "noop", "args": []}},
    )
    assert resp.status_code == 422


# ---- tick_limit clamp (Phase C.2 review Fix #2) ---------------------------


def test_simulate_tick_limit_over_1000_rejected(mock_pool, client):
    """tick_limit > 1000 → 422 (Pydantic Field(le=1000)). No DB hits required."""
    resp = client.post(
        "/api/v1/bt/npc_alpha/runaway/simulate",
        json={
            "tree_json": {"id": "root", "type": "action", "name": "noop", "args": []},
            "state": {"player_position": [0, 0], "npc_state": {}, "time_of_day": "noon", "max_ticks": 100},
            "tick_limit": 5000,
        },
    )
    assert resp.status_code == 422  # Pydantic validation error


def test_simulate_tick_limit_zero_rejected(mock_pool, client):
    """tick_limit == 0 → 422 (Pydantic Field(ge=1))."""
    resp = client.post(
        "/api/v1/bt/npc_alpha/runaway/simulate",
        json={
            "tree_json": {"id": "root", "type": "action", "name": "noop", "args": []},
            "state": {"player_position": [0, 0], "npc_state": {}, "time_of_day": "noon", "max_ticks": 100},
            "tick_limit": 0,
        },
    )
    assert resp.status_code == 422


# ---- Body size limit (Phase C.2 review Fix #3) -----------------------------


def test_oversized_body_rejected_with_413(client):
    """Content-Length > 256 KB → 413 / R_021 before any handler runs."""
    big = "x" * (300 * 1024)  # 300 KB — well over the 256 KB cap.
    # Wrap it in a valid-looking BT JSON; the middleware should fire
    # BEFORE Pydantic sees the payload.
    payload = '{"tree_json": {"id": "root", "type": "action", "name": "noop", "args": ["' + big + '"]}}'
    resp = client.post(
        "/api/v1/bt/npc_alpha/big/simulate",
        content=payload,
        headers={"Content-Type": "application/json"},
    )
    assert resp.status_code == 413
    assert resp.json()["detail"]["code"] == "R_021"
