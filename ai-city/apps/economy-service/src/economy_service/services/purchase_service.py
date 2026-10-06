# apps/economy-service/src/economy_service/services/purchase_service.py
"""NPC 商品购买 — 原子扣款 + stock 减一. Sink 留给 W3."""
from __future__ import annotations
import asyncpg
from economy_service.clients.kafka_producer import KafkaProducer, TOPIC_TX_COMPLETED
from economy_service.clients.redis_client import RedisClient
from economy_service.errors import (
    InsufficientBalance, ProductNotFound, ProductOutOfStock,
)

# Module-level clients (injected via set_clients()).
_kafka: KafkaProducer | None = None
_redis: RedisClient | None = None


def set_clients(
    kafka: KafkaProducer | None = None,
    redis: RedisClient | None = None,
) -> None:
    """Inject kafka + redis clients (used in app lifespan + tests)."""
    global _kafka, _redis
    _kafka = kafka
    _redis = redis


class PurchaseService:
    SINK_RATIO = 0.05  # 5% 自动 sink — 等 central_bank_ledger 落地

    def __init__(self, pool: asyncpg.Pool) -> None:
        self.pool = pool

    async def list_for_npc(self, npc_id: str) -> list[dict]:
        async with self.pool.acquire() as conn:
            rows = await conn.fetch(
                "SELECT id, npc_id, name, price_gold, price_token, stock "
                "FROM product WHERE npc_id = $1 AND enabled = true "
                "ORDER BY id",
                npc_id,
            )
        return [dict(r) for r in rows]

    async def purchase(
        self, user_id: str, product_id: int, currency: str,
        idempotency_key: str, trace_id: str | None = None,
    ) -> dict:
        async with self.pool.acquire() as conn:
            async with conn.transaction():
                product = await conn.fetchrow(
                    "SELECT id, npc_id, name, price_gold, price_token, stock, enabled "
                    "FROM product WHERE id = $1 FOR UPDATE",
                    product_id,
                )
                if product is None or not product["enabled"]:
                    raise ProductNotFound(f"product {product_id} not found / disabled")

                if product["stock"] is not None and product["stock"] <= 0:
                    raise ProductOutOfStock(f"product {product_id} out of stock")

                price = (
                    product["price_gold"] if currency == "gold"
                    else (product["price_token"] or 0)
                )
                if price <= 0:
                    raise ValueError(f"product {product_id} not sold in {currency}")

                wallet = await conn.fetchrow(
                    f"SELECT {currency}_balance AS bal FROM wallet "
                    f"WHERE user_id = $1 FOR UPDATE",
                    user_id,
                )
                if wallet is None:
                    raise ValueError(f"wallet not found: {user_id}")

                if wallet["bal"] < price:
                    raise InsufficientBalance(
                        f"{currency} balance {wallet['bal']} < {price}"
                    )

                new_bal = wallet["bal"] - price

                await conn.execute(
                    f"UPDATE wallet SET {currency}_balance = $1 "
                    f"WHERE user_id = $2",
                    new_bal, user_id,
                )
                if product["stock"] is not None:
                    await conn.execute(
                        "UPDATE product SET stock = stock - 1 WHERE id = $1",
                        product_id,
                    )

                # Sink code W3: insert into central_bank_ledger (deferred)
                # sink_amount = int(price * self.SINK_RATIO)
                # if currency == "gold" and sink_amount > 0:
                #     await conn.execute(
                #         "INSERT INTO central_bank_ledger ... ('sink', 'gold', $1, ...)",
                #         sink_amount,
                #     )

                await conn.execute(
                    "INSERT INTO transaction (tx_type, user_id, currency, "
                    "amount, balance_after, product_id, trace_id) "
                    "VALUES ('npc_purchase', $1, $2, $3, $4, $5, $6)",
                    user_id, currency, -price, new_bal, product_id, trace_id,
                )

                result = {
                    "user_id": user_id,
                    "product_id": product_id,
                    "currency": currency,
                    "amount_paid": price,
                    "balance_after": new_bal,
                    "sink_amount": 0,  # W3 will compute + persist
                }

        # Post-commit: fire-and-forget event + cache write
        # (outside the `async with` so we don't keep the conn alive past return)
        if _kafka is not None:
            await _kafka.send(TOPIC_TX_COMPLETED, {
                "user_id": user_id,
                "product_id": product_id,
                "currency": currency,
                "amount": -price,
                "balance_after": new_bal,
                "tx_type": "npc_purchase",
            })
        if _redis is not None:
            # 只更新付款维度 (gold 或 token) — 不要把另一个维度覆盖成 0
            cache_kwargs = {currency: new_bal}
            await _redis.cache_balance(user_id, **cache_kwargs)
        return result
