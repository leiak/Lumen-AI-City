# apps/economy-service/src/economy_service/schemas.py
"""Pydantic v2 models for economy-service."""
from __future__ import annotations
from enum import Enum
from pydantic import BaseModel, Field


class Currency(str, Enum):
    GOLD = "gold"
    TOKEN = "token"


class TxType(str, Enum):
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
    stock: int | None = None  # None = unlimited
    enabled: bool


class PurchaseRequest(BaseModel):
    user_id: str
    product_id: int = Field(gt=0)
    currency: Currency
    idempotency_key: str = Field(min_length=8, max_length=64)
    trace_id: str | None = None
