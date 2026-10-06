"""BT Editor API — FastAPI application entrypoint.

Phase C.2: replaces the 4-line Phase-1 stub with a real backend
(4 endpoints under ``/api/v1/bt``) backed by ``apps/agent-os/src/agent_os/bt/``
(C.1 GA) for validation + simulate, and asyncpg for PG persistence.

Lifespan: opens the asyncpg pool on startup so production hits a
warm pool; tests inject a mock via ``bt_editor_api.db.set_pool()`` and
do not run the lifespan (FastAPI's ``TestClient`` only triggers lifespan
when used as a context manager — we use it bare).
"""
from __future__ import annotations

from collections.abc import AsyncIterator
from contextlib import asynccontextmanager

from fastapi import FastAPI

from bt_editor_api import db as db_mod
from bt_editor_api.api.v1.bt import router as bt_router


@asynccontextmanager
async def lifespan(app: FastAPI) -> AsyncIterator[None]:
    """Open the asyncpg pool on startup; close it on shutdown."""
    pool = await db_mod.get_pool()
    app.state.pool = pool
    try:
        yield
    finally:
        await pool.close()


app = FastAPI(
    title="AI City - BT Editor API",
    version="2.0.0",
    lifespan=lifespan,
)
app.include_router(bt_router)


@app.get("/health")
async def health() -> dict:
    return {"status": "ok"}
