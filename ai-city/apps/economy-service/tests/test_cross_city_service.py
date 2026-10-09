from __future__ import annotations

from datetime import UTC, datetime, timedelta
from unittest.mock import AsyncMock, MagicMock
from uuid import uuid4

import pytest
from economy_service.errors import (
    CrossCityAmountOutOfRangeError,
    CrossCityIdempotencyConflictError,
    CrossCityInvalidStateError,
    CrossCitySameCityError,
    CrossCityTransferNotFoundError,
    CrossCityUnsupportedCurrencyError,
    CrossCityValidationError,
    InsufficientBalance,
)
from economy_service.schemas import Currency
from economy_service.services import cross_city_service
from economy_service.services.cross_city_service import CrossCityService


def transfer_row(**overrides):
    values = {
        "id": 1,
        "global_id": uuid4(),
        "direction": "outbound",
        "source_city_id": "alpha",
        "destination_city_id": "beta",
        "source_user_id": "alice",
        "destination_user_id": "bob",
        "currency": "gold",
        "amount": 100,
        "status": "reserved",
        "idempotency_key": "cross-key-1",
        "trace_id": "trace-1",
        "expires_at": datetime.now(UTC) + timedelta(seconds=600),
        "reserved_at": datetime.now(UTC),
        "credited_at": None,
        "settled_at": None,
        "refunded_at": None,
    }
    values.update(overrides)
    return values


class FakeConn:
    def __init__(self):
        self.rows = []
        self.wallet_balance = 500
        self.bridge_balance = 0
        self.calls = []
        self.transaction = MagicMock(return_value=_NoOpTransaction())

    async def fetchrow(self, sql, *args):
        self.calls.append(("fetchrow", sql, args))
        if "FROM wallet" in sql:
            return {"gold_balance": self.wallet_balance}
        if "FROM cross_city_transfer" in sql:
            if "FOR UPDATE" not in sql:
                return self.rows[0] if self.rows else None
            if self.rows:
                return self.rows[0]
            return None
        if "INSERT INTO cross_city_transfer" in sql:
            row = transfer_row(
                global_id=args[0],
                source_city_id=args[1],
                destination_city_id=args[2],
                source_user_id=args[3],
                destination_user_id=args[4],
                currency=args[5],
                amount=args[6],
                idempotency_key=args[7],
                trace_id=args[8],
                expires_at=args[9],
                reserved_at=args[10],
            )
            self.rows.append(row)
            return row
        return None

    async def fetchval(self, sql, *args):
        self.calls.append(("fetchval", sql, args))
        if "INSERT INTO bridge_position" in sql:
            self.bridge_balance += args[1]
            return self.bridge_balance
        if "UPDATE bridge_position" in sql:
            if self.bridge_balance < args[0]:
                return None
            self.bridge_balance -= args[0]
            return self.bridge_balance
        return None

    async def execute(self, sql, *args):
        self.calls.append(("execute", sql, args))
        if "UPDATE wallet SET gold_balance" in sql:
            self.wallet_balance = args[0]
        if "UPDATE cross_city_transfer" in sql:
            self.rows[0]["status"] = "refunded"
            self.rows[0]["refunded_at"] = datetime.now(UTC)
        return "OK"


class _NoOpTransaction:
    async def __aenter__(self):
        return self

    async def __aexit__(self, *args):
        return False


def pool_for(conn):
    pool = MagicMock()
    pool.acquire.return_value.__aenter__ = AsyncMock(return_value=conn)
    pool.acquire.return_value.__aexit__ = AsyncMock(return_value=False)
    return pool


def reserve_kwargs(**overrides):
    values = {
        "source_city_id": "alpha",
        "source_user_id": "alice",
        "destination_city_id": "beta",
        "destination_user_id": "bob",
        "currency": Currency.GOLD,
        "amount": 100,
        "idempotency_key": "cross-key-1",
        "trace_id": "trace-1",
    }
    values.update(overrides)
    return values


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "kwargs,error",
    [
        ({"source_city_id": ""}, CrossCityValidationError),
        ({"currency": Currency.TOKEN}, CrossCityUnsupportedCurrencyError),
        ({"amount": 0}, CrossCityAmountOutOfRangeError),
        ({"amount": 100001}, CrossCityAmountOutOfRangeError),
        ({"destination_city_id": "alpha"}, CrossCitySameCityError),
        ({"ttl_seconds": 0}, CrossCityValidationError),
    ],
)
async def test_reserve_contract_validation(kwargs, error):
    conn = FakeConn()
    with pytest.raises(error):
        await CrossCityService(pool_for(conn)).reserve(**reserve_kwargs(**kwargs))
    assert conn.calls == []


@pytest.mark.asyncio
async def test_reserve_debits_wallet_and_increments_bridge():
    conn = FakeConn()
    kafka = AsyncMock()
    cross_city_service._kafka = kafka
    result = await CrossCityService(pool_for(conn)).reserve(**reserve_kwargs())

    assert result["status"] == "reserved"
    assert result["direction"] == "outbound"
    assert conn.wallet_balance == 400
    assert conn.bridge_balance == 100
    assert any("INSERT INTO bridge_ledger_entry" in call[1] for call in conn.calls)
    assert any("INSERT INTO transaction" in call[1] for call in conn.calls)
    kafka.send.assert_awaited_once_with(
        "econ.crosscity.gold.reserved",
        {
            "global_id": result["global_id"],
            "source_city_id": "alpha",
            "destination_city_id": "beta",
            "source_user_id": "alice",
            "destination_user_id": "bob",
            "currency": "gold",
            "amount": 100,
            "trace_id": "trace-1",
        },
    )


@pytest.mark.asyncio
async def test_reserve_insufficient_writes_no_rows():
    conn = FakeConn()
    conn.wallet_balance = 50
    with pytest.raises(InsufficientBalance):
        await CrossCityService(pool_for(conn)).reserve(**reserve_kwargs())
    assert conn.rows == []
    assert conn.bridge_balance == 0


@pytest.mark.asyncio
async def test_reserve_duplicate_returns_existing_without_second_debit():
    conn = FakeConn()
    existing = transfer_row()
    conn.rows.append(existing)
    result = await CrossCityService(pool_for(conn)).reserve(**reserve_kwargs())

    assert result["global_id"] == str(existing["global_id"])
    assert conn.wallet_balance == 500
    assert conn.bridge_balance == 0


@pytest.mark.asyncio
async def test_reserve_duplicate_conflict():
    conn = FakeConn()
    conn.rows.append(transfer_row(amount=200))
    with pytest.raises(CrossCityIdempotencyConflictError):
        await CrossCityService(pool_for(conn)).reserve(**reserve_kwargs())


@pytest.mark.asyncio
async def test_refund_succeeds_after_expiry():
    conn = FakeConn()
    row = transfer_row(expires_at=datetime.now(UTC) - timedelta(seconds=1))
    conn.rows.append(row)
    conn.bridge_balance = row["amount"]
    kafka = AsyncMock()
    cross_city_service._kafka = kafka
    result = await CrossCityService(pool_for(conn)).refund(conn.rows[0]["global_id"])

    assert result["status"] == "refunded"
    assert conn.wallet_balance == 600
    assert conn.bridge_balance == 0
    kafka.send.assert_awaited_once()


@pytest.mark.asyncio
async def test_refund_rejects_active_or_settled_transfer():
    for row in (
        transfer_row(),
        transfer_row(status="settled", expires_at=datetime.now(UTC) - timedelta(seconds=1)),
    ):
        conn = FakeConn()
        conn.rows.append(row)
        with pytest.raises(CrossCityInvalidStateError):
            await CrossCityService(pool_for(conn)).refund(row["global_id"])


@pytest.mark.asyncio
async def test_refund_missing_bridge_position_fails():
    conn = FakeConn()
    conn.rows.append(transfer_row(expires_at=datetime.now(UTC) - timedelta(seconds=1)))
    conn.bridge_balance = -1
    with pytest.raises(CrossCityInvalidStateError):
        await CrossCityService(pool_for(conn)).refund(conn.rows[0]["global_id"])


@pytest.mark.asyncio
async def test_get_not_found():
    conn = FakeConn()
    assert await CrossCityService(pool_for(conn)).get(str(uuid4())) is None


@pytest.mark.asyncio
async def test_get_invalid_id():
    with pytest.raises(CrossCityValidationError):
        await CrossCityService(pool_for(FakeConn())).get("not-a-uuid")


@pytest.mark.asyncio
async def test_refund_not_found():
    conn = FakeConn()
    with pytest.raises(CrossCityTransferNotFoundError):
        await CrossCityService(pool_for(conn)).refund(str(uuid4()))
