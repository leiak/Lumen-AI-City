"""Cross-city gold transfer source and destination ledger operations."""
from __future__ import annotations

from datetime import UTC, datetime, timedelta
from uuid import UUID, uuid4

from asyncpg import Pool

from economy_service.clients.kafka_producer import KafkaProducer
from economy_service.errors import (
    CrossCityAmountOutOfRangeError,
    CrossCityIdempotencyConflictError,
    CrossCityInvalidStateError,
    CrossCitySameCityError,
    CrossCityTransferNotFoundError,
    CrossCityUnsupportedCurrencyError,
    CrossCityValidationError,
    InsufficientBalance,
    WalletNotFound,
)
from economy_service.schemas import Currency

_kafka: KafkaProducer | None = None


def set_clients(kafka: KafkaProducer | None = None) -> None:
    global _kafka
    _kafka = kafka


def _validate_contract(
    source_city_id: str,
    destination_city_id: str,
    source_user_id: str,
    destination_user_id: str,
    currency: Currency,
    amount: int,
    idempotency_key: str,
) -> None:
    if not all(
        (source_city_id, destination_city_id, source_user_id, destination_user_id, idempotency_key)
    ):
        raise CrossCityValidationError("cross-city transfer identifiers are required")
    if currency != Currency.GOLD:
        raise CrossCityUnsupportedCurrencyError(f"currency not enabled: {currency.value}")
    if amount <= 0 or amount > 100000:
        raise CrossCityAmountOutOfRangeError(f"amount out of range: {amount}")
    if source_city_id == destination_city_id:
        raise CrossCitySameCityError("source and destination city must differ")


def _response(row) -> dict:
    return {
        "global_id": str(row["global_id"]),
        "direction": row["direction"],
        "status": row["status"],
        "source_city_id": row["source_city_id"],
        "destination_city_id": row["destination_city_id"],
        "source_user_id": row["source_user_id"],
        "destination_user_id": row["destination_user_id"],
        "currency": row["currency"],
        "amount": row["amount"],
        "trace_id": row["trace_id"],
        "expires_at": row["expires_at"],
        "reserved_at": row["reserved_at"],
        "credited_at": row["credited_at"],
        "settled_at": row["settled_at"],
        "refunded_at": row["refunded_at"],
    }


def _assert_same_contract(row, **kwargs) -> None:
    expected = {
        "source_city_id": kwargs["source_city_id"],
        "destination_city_id": kwargs["destination_city_id"],
        "source_user_id": kwargs["source_user_id"],
        "destination_user_id": kwargs["destination_user_id"],
        "currency": kwargs["currency"].value,
        "amount": kwargs["amount"],
    }
    for field, value in expected.items():
        if row[field] != value:
            raise CrossCityIdempotencyConflictError(
                "idempotency key reused with different transfer payload"
            )


class CrossCityService:
    """Local ledger operations for one cross-city transfer leg."""

    def __init__(self, pool: Pool) -> None:
        self.pool = pool

    async def reserve(
        self,
        source_city_id: str,
        source_user_id: str,
        destination_city_id: str,
        destination_user_id: str,
        currency: Currency,
        amount: int,
        idempotency_key: str,
        trace_id: str | None = None,
        ttl_seconds: int = 600,
    ) -> dict:
        _validate_contract(
            source_city_id,
            destination_city_id,
            source_user_id,
            destination_user_id,
            currency,
            amount,
            idempotency_key,
        )
        if ttl_seconds <= 0:
            raise CrossCityValidationError("reservation TTL must be positive")

        now = datetime.now(UTC)
        expires_at = now + timedelta(seconds=ttl_seconds)
        global_id = uuid4()

        async with self.pool.acquire() as conn, conn.transaction():
            existing = await conn.fetchrow(
                """
                SELECT * FROM cross_city_transfer
                WHERE direction = 'outbound' AND idempotency_key = $1
                FOR UPDATE
                """,
                idempotency_key,
            )
            if existing is not None:
                _assert_same_contract(
                    existing,
                    source_city_id=source_city_id,
                    destination_city_id=destination_city_id,
                    source_user_id=source_user_id,
                    destination_user_id=destination_user_id,
                    currency=currency,
                    amount=amount,
                )
                return _response(existing)

            wallet = await conn.fetchrow(
                "SELECT gold_balance FROM wallet WHERE user_id = $1 FOR UPDATE",
                source_user_id,
            )
            if wallet is None:
                raise WalletNotFound(f"wallet not found: {source_user_id}")
            if wallet["gold_balance"] < amount:
                raise InsufficientBalance(
                    f"gold balance {wallet['gold_balance']} < {amount}"
                )

            transfer = await conn.fetchrow(
                """
                INSERT INTO cross_city_transfer (
                    global_id, direction, source_city_id, destination_city_id,
                    source_user_id, destination_user_id, currency, amount,
                    status, idempotency_key, trace_id, expires_at, reserved_at
                )
                VALUES (
                    $1, 'outbound', $2, $3, $4, $5, $6, $7,
                    'reserved', $8, $9, $10, $11
                )
                ON CONFLICT (direction, idempotency_key) DO NOTHING
                RETURNING *
                """,
                global_id,
                source_city_id,
                destination_city_id,
                source_user_id,
                destination_user_id,
                currency.value,
                amount,
                idempotency_key,
                trace_id,
                expires_at,
                now,
            )
            if transfer is None:
                raise CrossCityIdempotencyConflictError(
                    "cross-city transfer already exists with this idempotency key"
                )

            new_balance = wallet["gold_balance"] - amount
            await conn.execute(
                "UPDATE wallet SET gold_balance = $1 WHERE user_id = $2",
                new_balance,
                source_user_id,
            )
            bridge_balance = await conn.fetchval(
                """
                INSERT INTO bridge_position (
                    peer_city_id, direction, currency, balance
                )
                VALUES ($1, 'outbound', 'gold', $2)
                ON CONFLICT (peer_city_id, direction, currency) DO UPDATE
                SET balance = bridge_position.balance + EXCLUDED.balance,
                    updated_at = NOW()
                RETURNING balance
                """,
                destination_city_id,
                amount,
            )
            await conn.execute(
                """
                INSERT INTO bridge_ledger_entry (
                    transfer_global_id, direction, peer_city_id, currency,
                    amount, balance_after, reason, trace_id
                )
                VALUES ($1, 'outbound', $2, 'gold', $3, $4, 'reserve', $5)
                """,
                global_id,
                destination_city_id,
                amount,
                bridge_balance,
                trace_id,
            )
            await conn.execute(
                """
                INSERT INTO transaction (
                    tx_type, user_id, counterparty_id, currency, amount,
                    balance_after, trace_id
                )
                VALUES ('cross_city_out', $1, $2, 'gold', $3, $4, $5)
                """,
                source_user_id,
                destination_user_id,
                -amount,
                new_balance,
                trace_id,
            )

        if _kafka is not None:
            await _kafka.send(
                "econ.crosscity.gold.reserved",
                {
                    "global_id": str(global_id),
                    "source_city_id": source_city_id,
                    "destination_city_id": destination_city_id,
                    "source_user_id": source_user_id,
                    "destination_user_id": destination_user_id,
                    "currency": currency.value,
                    "amount": amount,
                    "trace_id": trace_id,
                },
            )
        return _response(transfer)

    async def get(self, global_id: str) -> dict | None:
        try:
            global_id = UUID(str(global_id))
        except ValueError as exc:
            raise CrossCityValidationError(f"invalid transfer id: {global_id}") from exc

        async with self.pool.acquire() as conn:
            row = await conn.fetchrow(
                """
                SELECT * FROM cross_city_transfer
                WHERE global_id = $1 AND direction = 'outbound'
                """,
                global_id,
            )
        return _response(row) if row is not None else None

    async def refund(self, global_id: str | UUID) -> dict:
        try:
            global_id = global_id if isinstance(global_id, UUID) else UUID(str(global_id))
        except ValueError as exc:
            raise CrossCityValidationError(f"invalid transfer id: {global_id}") from exc

        async with self.pool.acquire() as conn, conn.transaction():
            transfer = await conn.fetchrow(
                """
                SELECT * FROM cross_city_transfer
                WHERE global_id = $1 AND direction = 'outbound'
                FOR UPDATE
                """,
                global_id,
            )
            if transfer is None:
                raise CrossCityTransferNotFoundError(f"transfer not found: {global_id}")
            if transfer["status"] != "reserved":
                raise CrossCityInvalidStateError(
                    f"cannot refund transfer in status {transfer['status']}"
                )
            if transfer["expires_at"] > datetime.now(UTC):
                raise CrossCityInvalidStateError("reservation has not expired")

            wallet = await conn.fetchrow(
                "SELECT gold_balance FROM wallet WHERE user_id = $1 FOR UPDATE",
                transfer["source_user_id"],
            )
            if wallet is None:
                raise WalletNotFound(f"wallet not found: {transfer['source_user_id']}")
            new_balance = wallet["gold_balance"] + transfer["amount"]
            await conn.execute(
                "UPDATE wallet SET gold_balance = $1 WHERE user_id = $2",
                new_balance,
                transfer["source_user_id"],
            )
            bridge_balance = await conn.fetchval(
                """
                UPDATE bridge_position
                SET balance = balance - $1, updated_at = NOW()
                WHERE peer_city_id = $2 AND direction = 'outbound' AND currency = 'gold'
                RETURNING balance
                """,
                transfer["amount"],
                transfer["destination_city_id"],
            )
            if bridge_balance is None:
                raise CrossCityInvalidStateError("bridge position missing for outbound refund")

            await conn.execute(
                """
                UPDATE cross_city_transfer
                SET status = 'refunded', refunded_at = NOW(), updated_at = NOW()
                WHERE id = $1
                """,
                transfer["id"],
            )
            await conn.execute(
                """
                INSERT INTO bridge_ledger_entry (
                    transfer_global_id, direction, peer_city_id, currency,
                    amount, balance_after, reason, trace_id
                )
                VALUES ($1, 'outbound', $2, 'gold', $3, $4, 'refund', $5)
                """,
                global_id,
                transfer["destination_city_id"],
                -transfer["amount"],
                bridge_balance,
                transfer["trace_id"],
            )
            await conn.execute(
                """
                INSERT INTO transaction (
                    tx_type, user_id, counterparty_id, currency, amount,
                    balance_after, trace_id
                )
                VALUES ('cross_city_out', $1, $2, 'gold', $3, $4, $5)
                """,
                transfer["source_user_id"],
                transfer["destination_user_id"],
                transfer["amount"],
                new_balance,
                transfer["trace_id"],
            )

        if _kafka is not None:
            await _kafka.send(
                "econ.crosscity.gold.refunded",
                {
                    "global_id": str(global_id),
                    "source_city_id": transfer["source_city_id"],
                    "destination_city_id": transfer["destination_city_id"],
                    "source_user_id": transfer["source_user_id"],
                    "destination_user_id": transfer["destination_user_id"],
                    "currency": transfer["currency"],
                    "amount": transfer["amount"],
                    "trace_id": transfer["trace_id"],
                },
            )
        refreshed = await self.get(str(global_id))
        if refreshed is None:
            raise CrossCityTransferNotFoundError(f"transfer not found after refund: {global_id}")
        return refreshed
