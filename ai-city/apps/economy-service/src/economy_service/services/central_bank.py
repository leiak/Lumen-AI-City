# apps/economy-service/src/economy_service/services/central_bank.py
"""中央银行: emit formula + manual sink."""
from __future__ import annotations
import asyncpg
from economy_service.clients.kafka_producer import (
    KafkaProducer, TOPIC_GOLD_EMITTED, TOPIC_GOLD_SUNK,
)
from economy_service.clients.redis_client import RedisClient

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


class CentralBankService:
    BASE_PER_PLAYER = 100  # gold per player per emit cycle
    SINK_CAPACITY = 10000  # soft cap for emit ratio
    MAX_GOLD_PER_PLAYER = 100000  # hard cap

    def __init__(self, pool: asyncpg.Pool) -> None:
        self.pool = pool

    async def compute_emit(self) -> tuple[int, int]:
        """Returns (total_amount_to_distribute, active_player_count).

        Per-player amount = total_amount // active_count (computed in emit()).
        """
        async with self.pool.acquire() as conn:
            active_count = await conn.fetchval(
                "SELECT COUNT(DISTINCT user_id) FROM wallet "
                "WHERE updated_at > NOW() - INTERVAL '24 hours'",
            )
            active_count = active_count or 1

            total_gold = await conn.fetchval(
                "SELECT COALESCE(SUM(gold_balance), 0) FROM wallet",
            )

        # 反通胀公式: 高存量少发
        ratio = max(0.0, min(1.0, 1.0 - total_gold / self.SINK_CAPACITY))
        amount = int(self.BASE_PER_PLAYER * active_count * ratio)
        return amount, active_count

    async def emit(self, reason: str = "scheduled_emit") -> dict:
        amount, active_count = await self.compute_emit()
        if amount <= 0:
            return {"emitted": 0, "active_players": active_count, "skipped": True}

        per_player = amount // active_count
        if per_player <= 0:
            return {"emitted": 0, "active_players": active_count, "skipped": True}

        async with self.pool.acquire() as conn:
            async with conn.transaction():
                await conn.execute(
                    "UPDATE wallet SET gold_balance = LEAST(gold_balance + $1, $2) "
                    "WHERE updated_at > NOW() - INTERVAL '24 hours'",
                    per_player, self.MAX_GOLD_PER_PLAYER,
                )
                await conn.execute(
                    "INSERT INTO central_bank_ledger "
                    "(event_type, currency, amount, reason) "
                    "VALUES ('emit', 'gold', $1, $2)",
                    per_player, reason,
                )

        result = {
            "emitted": per_player,
            "active_players": active_count,
            "total_distributed": per_player * active_count,
        }

        # Post-commit: fire-and-forget event (outside the with block)
        if _kafka is not None:
            await _kafka.send(TOPIC_GOLD_EMITTED, {
                "per_player": per_player,
                "active_players": active_count,
                "total_distributed": per_player * active_count,
                "reason": reason,
            })
        return result

    async def sink(self, user_id: str, amount: int, reason: str = "admin") -> dict:
        """Manual sink: 销毁 user_id 的 gold_balance amount。"""
        if amount <= 0:
            raise ValueError("amount must be > 0")

        async with self.pool.acquire() as conn:
            async with conn.transaction():
                bal = await conn.fetchval(
                    "SELECT gold_balance FROM wallet WHERE user_id = $1 FOR UPDATE",
                    user_id,
                )
                if bal is None:
                    raise ValueError(f"wallet not found: {user_id}")
                if bal < amount:
                    raise ValueError(f"insufficient: {bal} < {amount}")

                new_bal = bal - amount
                await conn.execute(
                    "UPDATE wallet SET gold_balance = $1 WHERE user_id = $2",
                    new_bal, user_id,
                )
                await conn.execute(
                    "INSERT INTO central_bank_ledger "
                    "(event_type, currency, amount, reason, trigger_user_id) "
                    "VALUES ('sink', 'gold', $1, $2, $3)",
                    amount, reason, user_id,
                )

        result = {"user_id": user_id, "sunk": amount, "balance_after": new_bal}

        # Post-commit: fire-and-forget event + cache invalidate
        if _kafka is not None:
            await _kafka.send(TOPIC_GOLD_SUNK, {
                "user_id": user_id,
                "amount": amount,
                "balance_after": new_bal,
                "reason": reason,
            })
        if _redis is not None:
            # sink 只动 gold — 不要覆盖 token
            await _redis.cache_balance(user_id, gold=new_bal)
        return result
