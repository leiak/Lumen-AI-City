# apps/economy-service/src/economy_service/services/wallet_service.py
"""Wallet queries + transfer logic."""
from __future__ import annotations

import asyncpg

from economy_service.errors import InsufficientBalance, TransferSelf, WalletNotFound
from economy_service.schemas import Currency


class WalletService:
    def __init__(self, pool: asyncpg.Pool) -> None:
        self.pool = pool

    async def get_wallet(self, user_id: str) -> dict | None:
        async with self.pool.acquire() as conn:
            return await conn.fetchrow(
                "SELECT user_id, gold_balance, token_balance, "
                "created_at, updated_at FROM wallet WHERE user_id = $1",
                user_id,
            )

    async def transfer(
        self, from_user_id: str, to_user_id: str,
        currency: Currency, amount: int, trace_id: str | None = None,
    ) -> tuple[dict, dict]:
        """Atomic transfer. Returns (from_wallet, to_wallet) after commit."""
        if from_user_id == to_user_id:
            raise TransferSelf("cannot transfer to self")

        async with self.pool.acquire() as conn, conn.transaction():
            # SELECT FOR UPDATE on both rows
            from_row = await conn.fetchrow(
                "SELECT gold_balance, token_balance FROM wallet "
                "WHERE user_id = $1 FOR UPDATE",
                from_user_id,
            )
            if from_row is None:
                raise WalletNotFound(f"wallet not found: {from_user_id}")

            from_balance = from_row[currency.value + "_balance"]
            if from_balance < amount:
                raise InsufficientBalance(
                    f"{currency} balance {from_balance} < {amount}"
                )

            # Idempotent create-or-find to_wallet
            await conn.execute(
                "INSERT INTO wallet (user_id) VALUES ($1) "
                "ON CONFLICT (user_id) DO NOTHING",
                to_user_id,
            )
            to_row = await conn.fetchrow(
                "SELECT gold_balance, token_balance FROM wallet "
                "WHERE user_id = $1 FOR UPDATE",
                to_user_id,
            )

            new_from = from_balance - amount
            new_to = to_row[currency.value + "_balance"] + amount

            await conn.execute(
                f"UPDATE wallet SET {currency.value}_balance = $1 "
                "WHERE user_id = $2",
                new_from, from_user_id,
            )
            await conn.execute(
                f"UPDATE wallet SET {currency.value}_balance = $1 "
                "WHERE user_id = $2",
                new_to, to_user_id,
            )

            # Append-only transaction log (2 rows)
            await conn.execute(
                "INSERT INTO transaction (tx_type, user_id, counterparty_id, "
                "currency, amount, balance_after, trace_id) "
                "VALUES ('player_transfer', $1, $2, $3, $4, $5, $6)",
                from_user_id, to_user_id, currency.value, -amount, new_from, trace_id,
            )
            await conn.execute(
                "INSERT INTO transaction (tx_type, user_id, counterparty_id, "
                "currency, amount, balance_after, trace_id) "
                "VALUES ('player_transfer', $1, $2, $3, $4, $5, $6)",
                to_user_id, from_user_id, currency.value, amount, new_to, trace_id,
            )

            return (
                {"user_id": from_user_id, currency.value + "_balance": new_from},
                {"user_id": to_user_id, currency.value + "_balance": new_to},
            )
