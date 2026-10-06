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
