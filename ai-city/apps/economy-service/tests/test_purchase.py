import pytest
from economy_service.services.purchase_service import PurchaseService
from economy_service.errors import (
    InsufficientBalance, ProductNotFoundError, ProductOutOfStockError,
)


@pytest.mark.asyncio
async def test_purchase_deducts_gold_and_decrements_stock(pool):
    """购买价 50 → balance 1000-50-2(sink)=948. (W5.4 auto-sink 5% enabled.)"""
    pool._wallets["alice"] = (1000, 0)
    pool._conn.fetchrow.side_effect = [
        {"id": 1, "npc_id": "npc_wang_boss_001", "name": "招牌红烧肉",
         "price_gold": 50, "price_token": None, "stock": 10, "enabled": True},
        {"bal": 1000},
    ]
    svc = PurchaseService(pool)
    result = await svc.purchase("alice", 1, "gold", "key-1")
    assert result["balance_after"] == 948  # 1000 - 50 (price) - 2 (5% sink)
    assert result["amount_paid"] == 50
    assert result["sink_amount"] == 2  # int(50 * 0.05) = 2


@pytest.mark.asyncio
async def test_purchase_auto_sinks_5_percent_to_central_bank(pool):
    """Auto-sink: price=200 → sink=10 → INSERT INTO central_bank_ledger (sink).

    验证：balance_after = 1000 - 200 - 10 = 790。
    conftest mock 的 ``execute_default`` 对 INSERT INTO central_bank_ledger
    落到 default "OK" 分支；我们检查 call args 拿到 sink_amount=10 且
    SQL 包含 'INSERT INTO central_bank_ledger' + "'sink'"。
    """
    pool._wallets["alice"] = (1000, 0)
    pool._conn.fetchrow.side_effect = [
        {"id": 1, "npc_id": "npc_wang_boss_001", "name": "招牌红烧肉",
         "price_gold": 200, "price_token": None, "stock": 10, "enabled": True},
        {"bal": 1000},
    ]
    svc = PurchaseService(pool)
    result = await svc.purchase("alice", 1, "gold", "key-sink-1")

    assert result["sink_amount"] == 10  # int(200 * 0.05)
    assert result["balance_after"] == 790  # 1000 - 200 - 10

    # 验证 INSERT INTO central_bank_ledger 被调用（amount=10, reason=auto_npc_purchase_sink）
    calls = pool._conn.execute.call_args_list
    sink_call_found = False
    for call in calls:
        sql = call.args[0]
        if "INSERT INTO central_bank_ledger" in sql and "'sink'" in sql:
            # args order: (sql, sink_amount, user_id)
            assert call.args[1] == 10, f"sink_amount wrong: {call.args[1]}"
            assert call.args[2] == "alice", f"trigger_user_id wrong: {call.args[2]}"
            sink_call_found = True
            break
    assert sink_call_found, (
        "expected INSERT INTO central_bank_ledger ('sink', ...) call not found "
        f"in {len(calls)} execute() calls"
    )


@pytest.mark.asyncio
async def test_purchase_token_currency_skips_sink(pool):
    """Token 维度购买不触发 sink（spec 仅谈"全系统金池"）。"""
    pool._wallets["alice"] = (1000, 500)
    pool._conn.fetchrow.side_effect = [
        {"id": 1, "npc_id": "npc_wang_boss_001", "name": "招牌红烧肉",
         "price_gold": None, "price_token": 100, "stock": 10, "enabled": True},
        {"bal": 500},  # token_balance
    ]
    svc = PurchaseService(pool)
    result = await svc.purchase("alice", 1, "token", "key-tok-1")

    assert result["balance_after"] == 400  # 500 - 100 (no sink on token)
    assert result["sink_amount"] == 0


@pytest.mark.asyncio
async def test_purchase_out_of_stock_raises(pool):
    pool._wallets["alice"] = (1000, 0)
    pool._conn.fetchrow.side_effect = [
        {"id": 1, "npc_id": "npc_wang_boss_001", "name": "招牌红烧肉",
         "price_gold": 50, "price_token": None, "stock": 0, "enabled": True},
    ]
    svc = PurchaseService(pool)
    with pytest.raises(ProductOutOfStockError):
        await svc.purchase("alice", 1, "gold", "key-2")


@pytest.mark.asyncio
async def test_purchase_not_found_raises(pool):
    pool._wallets["alice"] = (1000, 0)
    pool._conn.fetchrow.side_effect = [
        None,  # product not found
    ]
    svc = PurchaseService(pool)
    with pytest.raises(ProductNotFoundError):
        await svc.purchase("alice", 999, "gold", "key-3")


@pytest.mark.asyncio
async def test_purchase_insufficient_raises(pool):
    """余额 10 买 50 gold → total_debit 50+2=52 > 10 → InsufficientBalance."""
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
    with pytest.raises(ProductNotFoundError):
        await svc.purchase("alice", 1, "gold", "key-5")
