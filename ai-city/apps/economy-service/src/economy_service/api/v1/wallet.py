# apps/economy-service/src/economy_service/api/v1/wallet.py
from __future__ import annotations
import os
import redis.asyncio as redis
from fastapi import APIRouter, HTTPException

from economy_service.db import get_pool
from economy_service.errors import EconomyError, WalletNotFound
from economy_service.schemas import (
    Currency, ErrorResponse, TransferRequest, WalletResponse,
)
from economy_service.services.idempotency import IdempotencyStore
from economy_service.services.wallet_service import WalletService

router = APIRouter(prefix="/api/v1/wallet", tags=["wallet"])


def _redis() -> redis.Redis:
    return redis.from_url(
        os.environ.get("REDIS_URL", "redis://redis:6379/0"),
    )


@router.get("/{user_id}", response_model=WalletResponse)
async def get_wallet(user_id: str) -> WalletResponse:
    svc = WalletService(await get_pool())
    row = await svc.get_wallet(user_id)
    if row is None:
        raise HTTPException(404, detail={"code": "R_404", "msg": "wallet not found"})
    return WalletResponse(
        user_id=row["user_id"],
        gold_balance=row["gold_balance"],
        token_balance=row["token_balance"],
        created_at=str(row["created_at"]),
        updated_at=str(row["updated_at"]),
    )


@router.post("/transfer", response_model=WalletResponse)
async def transfer(req: TransferRequest) -> WalletResponse:
    idem = IdempotencyStore(_redis())
    cached = await idem.get_cached("transfer", req.idempotency_key)
    if cached:
        return WalletResponse(**cached)

    svc = WalletService(await get_pool())
    try:
        from_w, _to_w = await svc.transfer(
            req.from_user_id, req.to_user_id,
            req.currency, req.amount, trace_id=req.memo,
        )
    except EconomyError as e:
        raise HTTPException(e.http_status, detail={"code": e.code, "msg": e.msg})

    # Cache for idempotent retry
    cached_resp = {
        "user_id": from_w["user_id"],
        "gold_balance": from_w.get("gold_balance", 0),
        "token_balance": from_w.get("token_balance", 0),
        "created_at": "2026-10-06T00:00:00Z",
        "updated_at": "2026-10-06T00:00:00Z",
    }
    await idem.check_and_set("transfer", req.idempotency_key, cached_resp)

    return WalletResponse(**cached_resp)