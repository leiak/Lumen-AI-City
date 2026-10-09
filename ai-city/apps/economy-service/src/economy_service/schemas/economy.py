"""Core economy schemas."""

from enum import StrEnum

from datetime import datetime
from uuid import UUID

from pydantic import BaseModel, Field


class Currency(StrEnum):
    GOLD = "gold"
    TOKEN = "token"


class TxType(StrEnum):
    PLAYER_TRANSFER = "player_transfer"
    NPC_PURCHASE = "npc_purchase"
    CENTRAL_BANK_EMIT = "central_bank_emit"
    NPC_SINK = "npc_sink"


class WalletResponse(BaseModel):
    user_id: str
    gold_balance: int = Field(ge=0)
    token_balance: int = Field(ge=0)
    created_at: str
    updated_at: str


class TransferRequest(BaseModel):
    from_user_id: str
    to_user_id: str
    currency: Currency
    amount: int = Field(gt=0, le=100000)
    idempotency_key: str = Field(min_length=8, max_length=64)
    memo: str | None = Field(default=None, max_length=200)


class TxRecord(BaseModel):
    id: int
    tx_type: TxType
    user_id: str
    counterparty_id: str | None = None
    currency: Currency
    amount: int
    balance_after: int
    product_id: int | None = None
    trace_id: str | None = None
    created_at: str


class TxListResponse(BaseModel):
    user_id: str
    transactions: list[TxRecord]
    total: int


class ErrorResponse(BaseModel):
    detail: dict


class Product(BaseModel):
    id: int
    npc_id: str
    name: str
    price_gold: int = Field(ge=0)
    price_token: int | None = Field(default=None, ge=0)
    stock: int | None = None
    enabled: bool


class PurchaseRequest(BaseModel):
    user_id: str
    product_id: int = Field(gt=0)
    currency: Currency
    idempotency_key: str = Field(min_length=8, max_length=64)
    trace_id: str | None = None


class SinkRequest(BaseModel):
    user_id: str = Field(min_length=1)
    amount: int = Field(gt=0)
    reason: str = "admin"


class CrossCityTransferRequest(BaseModel):
    source_city_id: str = Field(min_length=1, max_length=64)
    source_user_id: str = Field(min_length=1, max_length=64)
    destination_city_id: str = Field(min_length=1, max_length=64)
    destination_user_id: str = Field(min_length=1, max_length=64)
    currency: Currency = Currency.GOLD
    amount: int = Field(gt=0, le=100000)
    idempotency_key: str = Field(min_length=8, max_length=64)
    trace_id: str | None = Field(default=None, max_length=200)


class CrossCityTransferResponse(BaseModel):
    global_id: str
    direction: str
    status: str
    source_city_id: str
    destination_city_id: str
    source_user_id: str
    destination_user_id: str
    currency: str
    amount: int
    trace_id: str | None = None
    expires_at: datetime
    reserved_at: datetime | None = None
    credited_at: datetime | None = None
    settled_at: datetime | None = None
    refunded_at: datetime | None = None


class CrossCityCreditRequest(BaseModel):
    global_id: UUID
    source_city_id: str = Field(min_length=1, max_length=64)
    source_user_id: str = Field(min_length=1, max_length=64)
    destination_city_id: str = Field(min_length=1, max_length=64)
    destination_user_id: str = Field(min_length=1, max_length=64)
    currency: Currency = Currency.GOLD
    amount: int = Field(gt=0, le=100000)
    idempotency_key: str = Field(min_length=8, max_length=64)
    trace_id: str | None = Field(default=None, max_length=200)
    reserved_at: datetime
    expires_at: datetime
