"""Economy Service FastAPI app."""
from __future__ import annotations

from contextlib import asynccontextmanager

from fastapi import FastAPI, Request
from fastapi.responses import JSONResponse

from economy_service.api.v1.admin import router as admin_router
from economy_service.api.v1.marketplace.npc_templates import router as npc_templates_router
from economy_service.api.v1.marketplace.saga_templates import router as saga_templates_router
from economy_service.api.v1.products import router as products_router
from economy_service.api.v1.transactions import router as transactions_router
from economy_service.api.v1.wallet import router as wallet_router
from economy_service.clients.kafka_producer import KafkaProducer
from economy_service.clients.redis_client import RedisClient
from economy_service.db import close_pool, get_pool
from economy_service.errors import EconomyError
from economy_service.scheduler import start_scheduler
from economy_service.services import (
    central_bank,
    marketplace_service,
    purchase_service,
    wallet_service,
)


@asynccontextmanager
async def lifespan(app: FastAPI):
    await get_pool()  # warm up pool
    # Wire Kafka + Redis cache clients into services (no-op if startup fails).
    kafka_producer = KafkaProducer()
    redis_client = RedisClient()
    await kafka_producer.start()
    await redis_client.start()
    wallet_service.set_clients(kafka=kafka_producer, redis=redis_client)
    purchase_service.set_clients(kafka=kafka_producer, redis=redis_client)
    marketplace_service.set_clients(kafka=kafka_producer)
    central_bank.set_clients(kafka=kafka_producer, redis=redis_client)
    task = start_scheduler()  # 启动中央银行 scheduler
    try:
        yield
    finally:
        task.cancel()
        await kafka_producer.stop()
        await redis_client.stop()
        await close_pool()


app = FastAPI(title="AI City - Economy Service", lifespan=lifespan)


@app.get("/health")
async def health():
    return {"status": "ok"}


app.include_router(wallet_router)
app.include_router(products_router)
app.include_router(admin_router)
app.include_router(transactions_router)
app.include_router(npc_templates_router)
app.include_router(saga_templates_router)


@app.exception_handler(EconomyError)
async def economy_error_handler(req: Request, exc: EconomyError):
    return JSONResponse(
        status_code=exc.http_status,
        content={"detail": {"code": exc.code, "msg": exc.msg}},
    )
