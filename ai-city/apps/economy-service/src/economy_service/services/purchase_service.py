# apps/economy-service/src/economy_service/services/purchase_service.py
"""NPC 商品购买 — 原子扣款 + stock 减一 + 5% 自动 sink → central_bank_ledger.

Spec §中央银行机制 (自动 sink): 每次 NPC 售货自动从买家 gold 扣
``NPC_SINK_RATIO * price`` 沉淀 → central_bank_ledger ('sink' event)。
Sink 比例可通过 env ``NPC_SINK_RATIO`` 覆盖（默认 0.05 = 5%）。
"""
from __future__ import annotations
import os
import asyncpg
from economy_service.clients.kafka_producer import (
    KafkaProducer, TOPIC_TX_COMPLETED, TOPIC_GOLD_SUNK,
)
from economy_service.clients.redis_client import RedisClient
from economy_service.errors import (
    InsufficientBalance, ProductNotFoundError, ProductOutOfStockError,
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
    # 自动 sink 比例 — spec §自动 sink (默认 5% of price).
    # env override via NPC_SINK_RATIO; e.g. ``NPC_SINK_RATIO=0.10`` 关闭测试压测。
    # 仅 gold 维度生效（"全系统金池"）。
    SINK_RATIO = float(os.environ.get("NPC_SINK_RATIO", "0.05"))

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
                    raise ProductNotFoundError(f"product {product_id} not found / disabled")

                if product["stock"] is not None and product["stock"] <= 0:
                    raise ProductOutOfStockError(f"product {product_id} out of stock")

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

                # Auto-sink: 每次 NPC gold 售货沉淀 price * SINK_RATIO → 销毁。
                # token 维度暂不触发 sink（spec 只谈"金池"）。
                sink_amount = (
                    int(price * self.SINK_RATIO) if currency == "gold" else 0
                )
                total_debit = price + sink_amount

                if wallet["bal"] < total_debit:
                    raise InsufficientBalance(
                        f"{currency} balance {wallet['bal']} < {total_debit} "
                        f"(price={price} + sink={sink_amount})"
                    )

                new_bal = wallet["bal"] - total_debit

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

                # 写 central_bank_ledger 1 行（spec §Sink 触发）
                if sink_amount > 0:
                    await conn.execute(
                        "INSERT INTO central_bank_ledger "
                        "(event_type, currency, amount, reason, trigger_user_id) "
                        "VALUES ('sink', 'gold', $1, 'auto_npc_purchase_sink', $2)",
                        sink_amount, user_id,
                    )

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
                    "sink_amount": sink_amount,
                }

        # Post-commit: fire-and-forget events + cache write
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
            if sink_amount > 0:
                # Kafka emit `gold.sunk`（spec §Sink 触发）
                await _kafka.send(TOPIC_GOLD_SUNK, {
                    "user_id": user_id,
                    "amount": sink_amount,
                    "balance_after": new_bal,
                    "reason": "auto_npc_purchase_sink",
                })
        if _redis is not None:
            # 只更新付款维度 (gold 或 token) — 不要把另一个维度覆盖成 0
            cache_kwargs = {currency: new_bal}
            await _redis.cache_balance(user_id, **cache_kwargs)
        return result
