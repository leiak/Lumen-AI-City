"""Economy Service FastAPI app."""
from __future__ import annotations
from contextlib import asynccontextmanager
from fastapi import FastAPI, Request
from fastapi.responses import JSONResponse

from economy_service.api.v1.admin import router as admin_router
from economy_service.api.v1.products import router as products_router
from economy_service.api.v1.wallet import router as wallet_router
from economy_service.db import close_pool, get_pool
from economy_service.errors import EconomyError
from economy_service.scheduler import start_scheduler


@asynccontextmanager
async def lifespan(app: FastAPI):
    await get_pool()  # warm up pool
    task = start_scheduler()  # NEW: 启动中央银行 scheduler
    yield
    task.cancel()
    await close_pool()


app = FastAPI(title="AI City - Economy Service", lifespan=lifespan)


@app.get("/health")
async def health():
    return {"status": "ok"}


app.include_router(wallet_router)
app.include_router(products_router)
app.include_router(admin_router)


@app.exception_handler(EconomyError)
async def economy_error_handler(req: Request, exc: EconomyError):
    return JSONResponse(
        status_code=exc.http_status,
        content={"detail": {"code": exc.code, "msg": exc.msg}},
    )