from __future__ import annotations

from datetime import UTC, datetime, timedelta
from unittest.mock import AsyncMock, MagicMock
from uuid import uuid4

import pytest
from economy_service.errors import (
    CrossCityAmountOutOfRangeError,
    CrossCityExpiredError,
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
        self.destination_balance = 0
        self.bridge_balance = 0
        self.calls = []
        self.inbound_row = None
        self.transaction = MagicMock(return_value=_NoOpTransaction())

    async def fetchrow(self, sql, *args):
        self.calls.append(("fetchrow", sql, args))
        if "FROM wallet" in sql:
            return {"gold_balance": self.wallet_balance}
        if "FROM cross_city_transfer" in sql:
            if "direction = 'inbound' AND idempotency_key" in sql:
                return self.inbound_row
            if "direction = 'inbound'" in sql:
                return self.inbound_row
            if "FOR UPDATE" not in sql:
                return self.rows[0] if self.rows else None
            if self.rows:
                return self.rows[0]
            return None
        if "INSERT INTO cross_city_transfer" in sql:
            if "'inbound'" in sql:
                row = transfer_row(
                    global_id=args[0],
                    direction="inbound",
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
                    status="credited",
                    credited_at=datetime.now(UTC),
                )
                self.inbound_row = row
            else:
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
            self.rows.append(row)
            return row
        return None

    async def fetch(self, sql, *args):
        self.calls.append(("fetch", sql, args))
        if "direction = 'outbound'" in sql and "expires_at <= NOW()" in sql:
            now = datetime.now(UTC)
            return [row for row in self.rows if row["status"] == "reserved" and row["expires_at"] <= now]
        return []

    async def fetchval(self, sql, *args):
        self.calls.append(("fetchval", sql, args))
        if "INSERT INTO wallet" in sql:
            self.destination_balance += args[1]
            return self.destination_balance
        if "UPDATE wallet" in sql:
            self.destination_balance += args[0]
            return self.destination_balance
        if "INSERT INTO bridge_position" in sql:
            if "'inbound'" in sql:
                self.bridge_balance -= abs(args[1])
            else:
                self.bridge_balance += args[1]
            return self.bridge_balance
        if "UPDATE bridge_position" in sql:
            if "'inbound'" in sql:
                self.bridge_balance -= abs(args[0])
            else:
                if self.bridge_balance < args[0]:
                    return None
                self.bridge_balance -= args[0]
            return self.bridge_balance
        return None

    async def execute(self, sql, *args):
        self.calls.append(("execute", sql, args))
        if "UPDATE wallet SET gold_balance" in sql:
            if "gold_balance + $1" in sql:
                self.destination_balance += args[0]
            else:
                self.wallet_balance = args[0]
        if "INSERT INTO cross_city_transfer" in sql and "'inbound'" in sql:
            self.inbound_row = transfer_row(
                global_id=args[0],
                direction="inbound",
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
                status="credited",
                credited_at=datetime.now(UTC),
            )
        if "UPDATE cross_city_transfer" in sql:
            if "status = 'settled'" in sql:
                self.rows[0]["status"] = "settled"
                self.rows[0]["settled_at"] = datetime.now(UTC)
            else:
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


def credit_kwargs(**overrides):
    now = datetime.now(UTC)
    values = {
        "global_id": str(uuid4()),
        "source_city_id": "alpha",
        "source_user_id": "alice",
        "destination_city_id": "beta",
        "destination_user_id": "bob",
        "currency": Currency.GOLD,
        "amount": 100,
        "idempotency_key": "inbound-key-1",
        "reserved_at": now,
        "expires_at": now + timedelta(seconds=600),
        "trace_id": "trace-credit",
    }
    values.update(overrides)
    return values


@pytest.mark.asyncio
async def test_credit_inbound_creates_wallet_and_credits_it():
    conn = FakeConn()
    kafka = AsyncMock()
    cross_city_service._kafka = kafka
    result = await CrossCityService(pool_for(conn)).credit_inbound(**credit_kwargs())

    assert result["direction"] == "inbound"
    assert result["status"] == "credited"
    assert conn.destination_balance == 100
    assert conn.bridge_balance == -100
    assert any("INSERT INTO transaction" in call[1] for call in conn.calls)
    kafka.send.assert_awaited_once_with(
        "econ.crosscity.gold.credited",
        {
            "global_id": result["global_id"],
            "source_city_id": "alpha",
            "destination_city_id": "beta",
            "source_user_id": "alice",
            "destination_user_id": "bob",
            "currency": "gold",
            "amount": 100,
            "trace_id": "trace-credit",
        },
    )


@pytest.mark.asyncio
async def test_credit_inbound_duplicate_returns_without_second_credit():
    conn = FakeConn()
    conn.inbound_row = transfer_row(
        direction="inbound",
        idempotency_key="inbound-key-1",
        status="credited",
    )
    result = await CrossCityService(pool_for(conn)).credit_inbound(**credit_kwargs())

    assert result["global_id"] == str(conn.inbound_row["global_id"])
    assert conn.destination_balance == 0
    assert conn.bridge_balance == 0


@pytest.mark.asyncio
async def test_credit_inbound_duplicate_contract_conflict():
    conn = FakeConn()
    conn.inbound_row = transfer_row(
        direction="inbound",
        idempotency_key="inbound-key-1",
        status="credited",
        amount=200,
    )
    with pytest.raises(CrossCityIdempotencyConflictError):
        await CrossCityService(pool_for(conn)).credit_inbound(**credit_kwargs())


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "kwargs",
    [
        {"currency": Currency.TOKEN},
        {"amount": 0},
        {"destination_city_id": "alpha"},
        {"reserved_at": datetime.now(UTC).replace(tzinfo=None)},
    ],
)
async def test_credit_inbound_contract_validation(kwargs):
    conn = FakeConn()
    with pytest.raises(
        (
            CrossCityUnsupportedCurrencyError,
            CrossCityAmountOutOfRangeError,
            CrossCitySameCityError,
            CrossCityValidationError,
        )
    ):
        await CrossCityService(pool_for(conn)).credit_inbound(**credit_kwargs(**kwargs))


@pytest.mark.asyncio
async def test_credit_inbound_rejects_expired_request():
    conn = FakeConn()
    expired = datetime.now(UTC) - timedelta(seconds=1)
    with pytest.raises(CrossCityExpiredError):
        await CrossCityService(pool_for(conn)).credit_inbound(
            **credit_kwargs(expires_at=expired)
        )
    assert conn.inbound_row is None
    assert conn.destination_balance == 0


@pytest.mark.asyncio
async def test_settle_outbound_marks_settled_without_balance_change():
    conn = FakeConn()
    row = transfer_row()
    conn.rows.append(row)
    original_bridge = conn.bridge_balance
    kafka = AsyncMock()
    cross_city_service._kafka = kafka
    result = await CrossCityService(pool_for(conn)).settle_outbound(row["global_id"])

    assert result["status"] == "settled"
    assert conn.wallet_balance == 500
    assert conn.bridge_balance == original_bridge
    kafka.send.assert_awaited_once_with(
        "econ.crosscity.gold.settled",
        {
            "global_id": str(row["global_id"]),
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
async def test_settle_outbound_rejects_refunded_leg():
    conn = FakeConn()
    row = transfer_row(status="refunded")
    conn.rows.append(row)
    with pytest.raises(CrossCityInvalidStateError):
        await CrossCityService(pool_for(conn)).settle_outbound(row["global_id"])


@pytest.mark.asyncio
async def test_settle_outbound_not_found():
    with pytest.raises(CrossCityTransferNotFoundError):
        await CrossCityService(pool_for(FakeConn())).settle_outbound(str(uuid4()))


@pytest.mark.asyncio
async def test_list_expired_outbound_returns_reserved_rows():
    conn = FakeConn()
    active = transfer_row()
    expired = transfer_row(expires_at=datetime.now(UTC) - timedelta(seconds=1))
    conn.rows.extend([active, expired])
    rows = await CrossCityService(pool_for(conn)).list_expired_outbound()

    assert [row["global_id"] for row in rows] == [str(expired["global_id"])]
