import pytest
from economy_service.services.purchase_service import PurchaseService
from economy_service.errors import (
    InsufficientBalance, ProductNotFound, ProductOutOfStock,
)


@pytest.mark.asyncio
async def test_purchase_deducts_gold_and_decrements_stock(pool):
    pool._wallets["alice"] = (1000, 0)
    pool._conn.fetchrow.side_effect = [
        {"id": 1, "npc_id": "npc_wang_boss_001", "name": "招牌红烧肉",
         "price_gold": 50, "price_token": None, "stock": 10, "enabled": True},
        {"bal": 1000},
    ]
    svc = PurchaseService(pool)
    result = await svc.purchase("alice", 1, "gold", "key-1")
    assert result["balance_after"] == 950
    assert result["amount_paid"] == 50


@pytest.mark.asyncio
async def test_purchase_out_of_stock_raises(pool):
    pool._wallets["alice"] = (1000, 0)
    pool._conn.fetchrow.side_effect = [
        {"id": 1, "npc_id": "npc_wang_boss_001", "name": "招牌红烧肉",
         "price_gold": 50, "price_token": None, "stock": 0, "enabled": True},
    ]
    svc = PurchaseService(pool)
    with pytest.raises(ProductOutOfStock):
        await svc.purchase("alice", 1, "gold", "key-2")


@pytest.mark.asyncio
async def test_purchase_not_found_raises(pool):
    pool._wallets["alice"] = (1000, 0)
    pool._conn.fetchrow.side_effect = [
        None,  # product not found
    ]
    svc = PurchaseService(pool)
    with pytest.raises(ProductNotFound):
        await svc.purchase("alice", 999, "gold", "key-3")


@pytest.mark.asyncio
async def test_purchase_insufficient_raises(pool):
    pool._wallets["alice"] = (10, 0)
    pool._conn.fetchrow.side_effect = [
        {"id": 1, "npc_id": "npc_wang_boss_001", "name": "招牌红烧肉",
         "price_gold": 50, "price_token": None, "stock": 10, "enabled": True},
        {"bal": 10},
    ]
    svc = PurchaseService(pool)
    with pytest.raises(InsufficientBalance):
        await svc.purchase("alice", 1, "gold", "key-4")


@pytest.mark.asyncio
async def test_purchase_disabled_raises_not_found(pool):
    pool._wallets["alice"] = (1000, 0)
    pool._conn.fetchrow.side_effect = [
        {"id": 1, "npc_id": "npc_wang_boss_001", "name": "招牌红烧肉",
         "price_gold": 50, "price_token": None, "stock": 10, "enabled": False},
    ]
    svc = PurchaseService(pool)
    with pytest.raises(ProductNotFound):
        await svc.purchase("alice", 1, "gold", "key-5")
