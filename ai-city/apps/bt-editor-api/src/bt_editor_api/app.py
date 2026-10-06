"""BT Editor API — FastAPI application entrypoint.

Phase C.2: replaces the 4-line Phase-1 stub with a real backend
(4 endpoints under ``/api/v1/bt``) backed by ``apps/agent-os/src/agent_os/bt/``
(C.1 GA) for validation + simulate, and asyncpg for PG persistence.

Lifespan: opens the asyncpg pool on startup so production hits a
warm pool; tests inject a mock via ``bt_editor_api.db.set_pool()`` and
do not run the lifespan (FastAPI's ``TestClient`` only triggers lifespan
when used as a context manager — we use it bare).

Middleware: ``BodySizeLimitMiddleware`` caps incoming request bodies at
256 KB so a malicious client can't OOM the process with a 1 GB POST.
Oversized bodies short-circuit with HTTP 413 / R_021 before any
downstream handler is invoked.
"""
from __future__ import annotations

from collections.abc import AsyncIterator
from contextlib import asynccontextmanager

from fastapi import FastAPI
from starlette.middleware.base import BaseHTTPMiddleware
from starlette.requests import Request
from starlette.responses import JSONResponse, Response

from bt_editor_api import db as db_mod
from bt_editor_api.api.v1.bt import router as bt_router

# ---- Middleware ------------------------------------------------------------


class BodySizeLimitMiddleware(BaseHTTPMiddleware):
    """Reject requests with a Content-Length header above ``MAX_BYTES``.

    The check fires BEFORE any handler runs, so a 1 GB POST can't pin
    the process by streaming the body into the JSON parser. Chunked /
    unknown-length requests are passed through — Starlette will stream
    them, and downstream layer size limits catch the worst case.
    """

    MAX_BYTES = 256 * 1024  # 256 KB — enough for the largest legal BT JSON.

    async def dispatch(self, request: Request, call_next: object) -> Response:
        cl = request.headers.get("content-length")
        if cl and cl.isdigit() and int(cl) > self.MAX_BYTES:
            return JSONResponse(
                status_code=413,
                content={
                    "detail": {
                        "code": "R_021",
                        "msg": f"body too large ({cl} > {self.MAX_BYTES})",
                    }
                },
            )
        return await call_next(request)  # type: ignore[arg-type]


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
# Body-size guard FIRST — runs before routing, so even /api/v1/bt paths
# with a malicious body get rejected before any handler code touches it.
app.add_middleware(BodySizeLimitMiddleware)
app.include_router(bt_router)


@app.get("/health")
async def health() -> dict:
    return {"status": "ok"}
