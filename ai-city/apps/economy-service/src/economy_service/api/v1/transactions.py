# apps/economy-service/src/economy_service/api/v1/transactions.py
"""GET /api/v1/transactions/{user_id} — player transaction history."""
from __future__ import annotations
from fastapi import APIRouter, Query

from economy_service.db import get_pool

router = APIRouter(prefix="/api/v1/transactions", tags=["transactions"])


@router.get("/{user_id}")
async def list_transactions(
    user_id: str,
    limit: int = Query(50, ge=1, le=200),
    offset: int = Query(0, ge=0),
) -> dict:
    """Return transaction history for `user_id` (newest first), paginated."""
    pool = await get_pool()
    async with pool.acquire() as conn:
        rows = await conn.fetch(
            """SELECT id, tx_type, currency, amount, balance_after,
                      counterparty_id, product_id, trace_id, created_at
               FROM transaction
               WHERE user_id = $1
               ORDER BY created_at DESC
               LIMIT $2 OFFSET $3""",
            user_id, limit, offset,
        )
        total = await conn.fetchval(
            "SELECT COUNT(*) FROM transaction WHERE user_id = $1",
            user_id,
        ) or 0
    return {
        "transactions": [
            {
                "id": r["id"],
                "tx_type": r["tx_type"],
                "currency": r["currency"],
                "amount": r["amount"],
                "balance_after": r["balance_after"],
                "counterparty_id": r["counterparty_id"],
                "product_id": r["product_id"],
                "trace_id": r["trace_id"],
                "created_at": r["created_at"].isoformat() if r["created_at"] else None,
            }
            for r in rows
        ],
        "total": total,
        "limit": limit,
        "offset": offset,
    }