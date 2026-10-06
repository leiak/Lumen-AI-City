# apps/economy-service/src/economy_service/api/v1/wallet.py
from __future__ import annotations
import os
import redis.asyncio as redis
from fastapi import APIRouter

from economy_service.db import get_pool
from economy_service.errors import WalletNotFound
from economy_service.schemas import (
    PurchaseRequest, TransferRequest, WalletResponse,
)
from economy_service.services.idempotency import IdempotencyStore
from economy_service.services.purchase_service import PurchaseService
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
        raise WalletNotFound(f"wallet not found: {user_id}")
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
    from_w, _to_w = await svc.transfer(
        req.from_user_id, req.to_user_id,
        req.currency, req.amount, trace_id=req.memo,
    )

    # Re-fetch full wallet to get real timestamps + both balances
    full = await svc.get_wallet(req.from_user_id)
    response = WalletResponse(
        user_id=full["user_id"],
        gold_balance=full["gold_balance"],
        token_balance=full["token_balance"],
        created_at=str(full["created_at"]),
        updated_at=str(full["updated_at"]),
    )
    await idem.check_and_set("transfer", req.idempotency_key, response.model_dump())
    return response


@router.post("/purchase")
async def purchase(req: PurchaseRequest) -> dict:
    idem = IdempotencyStore(_redis())
    cached = await idem.get_cached("purchase", req.idempotency_key)
    if cached:
        return cached

    svc = PurchaseService(await get_pool())
    result = await svc.purchase(
        req.user_id, req.product_id, req.currency.value,
        req.idempotency_key, trace_id=req.trace_id,
    )

    await idem.check_and_set("purchase", req.idempotency_key, result)
    return result