"""Phase C.2 — BT editor FastAPI router.

Four endpoints (base path ``/api/v1/bt``):

* ``GET    /{npc_id}``                  — list trees for an NPC
* ``GET    /{npc_id}/{tree_name}``      — fetch a single tree
* ``POST   /{npc_id}/{tree_name}``      — upsert a tree (validates + saves)
* ``POST   /{npc_id}/{tree_name}/simulate`` — dry-run tick with trace log

All endpoints share the module-level asyncpg pool from
``bt_editor_api.db.get_pool()``; tests override it via ``set_pool()``.
"""
from __future__ import annotations

import json
from typing import Any

from agent_os.bt.errors import BTError
from agent_os.bt.evaluator import tick
from agent_os.bt.schema import Status
from fastapi import APIRouter, HTTPException, Path

from bt_editor_api import db as db_mod
from bt_editor_api.schemas import (
    SaveTreeRequest,
    SimulateRequest,
    SimulateResponse,
    SimulateTraceEntry,
    TreeFull,
    TreeSummary,
)
from bt_editor_api.settings import BT_TREE_DEPTH_LIMIT, BT_TREE_NODE_LIMIT

router = APIRouter(prefix="/api/v1/bt", tags=["bt"])


# ---- SQL fragments ---------------------------------------------------------


_LIST_SQL = (
    "SELECT name, version, updated_at FROM bt_tree WHERE npc_id = $1 "
    "ORDER BY updated_at DESC"
)
_GET_SQL = (
    "SELECT npc_id, name, tree_json, version, created_at, updated_at "
    "FROM bt_tree WHERE npc_id = $1 AND name = $2"
)
# ON CONFLICT keeps the original id + created_at; trigger bumps version.
_UPSERT_SQL = (
    "INSERT INTO bt_tree (npc_id, name, tree_json) "
    "VALUES ($1, $2, $3::jsonb) "
    "ON CONFLICT (npc_id, name) DO UPDATE SET tree_json = EXCLUDED.tree_json"
)


# ---- Helpers ---------------------------------------------------------------


def _coerce_tree_json(raw: Any) -> dict:
    """asyncpg returns JSONB as a ``str``; the response wants a ``dict``."""
    if isinstance(raw, (dict, list)):
        return raw  # type: ignore[return-value]
    if isinstance(raw, str):
        return json.loads(raw)
    if isinstance(raw, (bytes, bytearray)):
        return json.loads(raw.decode("utf-8"))
    raise HTTPException(
        status_code=400,
        detail={
            "code": "R_019",
            "msg": f"BT_PARSE_FAIL: stored tree_json has unexpected type {type(raw).__name__}",
        },
    )


def _coerce_iso(raw: Any) -> str:
    """Return ISO-8601 string for a timestamp value.

    asyncpg returns ``datetime`` for ``TIMESTAMPTZ`` columns; tests inject
    pre-formatted ``str`` so we accept both shapes.
    """
    if hasattr(raw, "isoformat"):
        return raw.isoformat()
    return str(raw)


def _row_to_full(row: dict) -> TreeFull:
    return TreeFull(
        npc_id=row["npc_id"],
        name=row["name"],
        tree_json=_coerce_tree_json(row["tree_json"]),
        version=row["version"],
        created_at=_coerce_iso(row["created_at"]),
        updated_at=_coerce_iso(row["updated_at"]),
    )


# ---- Endpoints -------------------------------------------------------------


@router.get("/{npc_id}", response_model=list[TreeSummary])
async def list_trees(
    npc_id: str = Path(..., max_length=64, pattern=r"^[A-Za-z0-9_-]+$"),
) -> list[TreeSummary]:
    """List all BT trees for an NPC, newest-updated first.

    Empty list when the NPC has no saved trees — matches the
    ``admin-portal /bt-editor`` UI expectation of "empty state with a
    'create' button".

    ``npc_id`` is bounded to 64 chars and ``[A-Za-z0-9_-]+`` so path
    traversal / injection attempts (e.g. ``../etc/passwd``) are rejected
    by FastAPI with 422 before any DB call runs.
    """
    pool = await db_mod.get_pool()
    rows = await pool.fetch(_LIST_SQL, npc_id)
    return [
        TreeSummary(
            name=r["name"],
            version=r["version"],
            updated_at=_coerce_iso(r["updated_at"]),
        )
        for r in rows
    ]


@router.get("/{npc_id}/{tree_name}", response_model=TreeFull)
async def get_tree(
    npc_id: str = Path(..., max_length=64, pattern=r"^[A-Za-z0-9_-]+$"),
    tree_name: str = Path(..., max_length=128, pattern=r"^[A-Za-z0-9_-]+$"),
) -> TreeFull:
    """Fetch one BT tree. 404 if not found."""
    pool = await db_mod.get_pool()
    row = await pool.fetchrow(_GET_SQL, npc_id, tree_name)
    if row is None:
        raise HTTPException(status_code=404, detail="tree not found")
    return _row_to_full(dict(row))


@router.post("/{npc_id}/{tree_name}", response_model=TreeFull)
async def save_tree(
    npc_id: str = Path(..., max_length=64, pattern=r"^[A-Za-z0-9_-]+$"),
    tree_name: str = Path(..., max_length=128, pattern=r"^[A-Za-z0-9_-]+$"),
    body: SaveTreeRequest = ...,
) -> TreeFull:
    """Upsert a BT tree.

    Validates ``tree_json`` via ``db.validate_tree`` (which reuses the
    C.1 loader) before touching the DB. On UPDATE the version trigger
    bumps ``version`` + ``updated_at`` automatically.
    """
    db_mod.validate_tree(
        body.tree_json,
        depth_limit=BT_TREE_DEPTH_LIMIT,
        node_limit=BT_TREE_NODE_LIMIT,
    )

    pool = await db_mod.get_pool()
    await pool.execute(_UPSERT_SQL, npc_id, tree_name, json.dumps(body.tree_json))

    row = await pool.fetchrow(_GET_SQL, npc_id, tree_name)
    if row is None:
        # Should never happen — UPSERT just wrote a row. Guard anyway.
        raise HTTPException(status_code=500, detail="upsert failed to return row")
    return _row_to_full(dict(row))


@router.post("/{npc_id}/{tree_name}/simulate", response_model=SimulateResponse)
async def simulate_tree(
    npc_id: str = Path(..., max_length=64, pattern=r"^[A-Za-z0-9_-]+$"),
    tree_name: str = Path(..., max_length=128, pattern=r"^[A-Za-z0-9_-]+$"),
    body: SimulateRequest = ...,
) -> SimulateResponse:
    """Dry-run a BT tree against a synthetic state and return a tick trace.

    Loops ``tick(tree, state)`` up to ``tick_limit`` times (default 100,
    clamped to [1, 1000] by ``SimulateRequest.tick_limit``), stopping
    early on terminal status (success / failure).

    **Fatal eval errors** (e.g. ``max_ticks`` exceeded, subtree lookup
    with no registry) raise ``BTError``; per the spec this surfaces as
    **HTTP 500 / R_020** so the client sees a clear failure mode
    instead of a 200 with ``status="error"``. Non-fatal ticks keep the
    trace-driven 200 response.
    """
    tree = db_mod.validate_tree(
        body.tree_json,
        depth_limit=BT_TREE_DEPTH_LIMIT,
        node_limit=BT_TREE_NODE_LIMIT,
    )

    state = db_mod.build_bt_state(body.state.model_dump())
    # state.max_ticks already honours [1, 1000] via SimulateState's Field
    # constraints; no need to overwrite it from body.tick_limit (which
    # controls how many OUTER iterations we run, not the per-tick
    # budget).

    try:
        trace: list[SimulateTraceEntry] = []
        final_status: str = "running"
        for _ in range(body.tick_limit):
            state.tick_count = 0
            # Spec: any BTError here is a fatal eval failure → 500 / R_020.
            # Non-fatal ticks never raise, so a clean trace path stays 200.
            status: Status = tick(tree, state)

            trace.append(
                SimulateTraceEntry(
                    node_id=tree.root.id,
                    status=status.value,
                    tick_count=state.tick_count,
                )
            )

            if status in (Status.SUCCESS, Status.FAILURE):
                final_status = status.value
                break
        else:
            # tick_limit reached while still RUNNING — preserve "running" so
            # the UI knows the tree didn't terminate (vs "error").
            final_status = "running"
    except BTError as e:
        # Spec: simulate returns 500 / R_020 on fatal eval errors
        # (subtree registry absent, infinite recursion hit, etc.). The
        # partial trace is intentionally dropped — the spec is silent on
        # exposing it in the 500 path, and clients should retry with a
        # smaller tree rather than try to render a half-built trace.
        raise HTTPException(
            status_code=500,
            detail={"code": "R_020", "msg": f"BT_EVAL_FAIL: {e}"},
        ) from e

    return SimulateResponse(
        status=final_status,
        trace=trace,
        final_state=db_mod.state_to_dict(state),
    )


__all__ = ["router"]
