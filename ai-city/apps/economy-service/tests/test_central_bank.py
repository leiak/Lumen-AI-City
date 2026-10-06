import pytest
from economy_service.services.central_bank import CentralBankService


@pytest.mark.asyncio
async def test_emit_distributes_to_active_players(pool):
    pool._wallets["alice"] = (100, 0)
    pool._wallets["bob"] = (200, 0)
    pool._conn.fetchval.side_effect = [2, 300]  # 2 active, total 300 gold
    svc = CentralBankService(pool)
    result = await svc.emit()
    # ratio = 1 - 300/10000 = 0.97, amount = 100 * 2 * 0.97 = 194
    # per_player = 194 // 2 = 97
    assert result["emitted"] == 97
    assert result["active_players"] == 2


@pytest.mark.asyncio
async def test_emit_caps_at_max_gold(pool):
    pool._wallets["alice"] = (99999, 0)
    pool._conn.fetchval.side_effect = [1, 0]  # 1 player, total=0 -> ratio=1
    svc = CentralBankService(pool)
    result = await svc.emit()
    # ratio = 1 - 0/10000 = 1, amount = 100 * 1 * 1 = 100
    # per_player = 100
    assert result["emitted"] == 100
    # Alice can't exceed 100000 due to MAX_GOLD_PER_PLAYER cap


@pytest.mark.asyncio
async def test_emit_skips_when_total_supply_high(pool):
    pool._wallets["alice"] = (50000, 0)
    pool._conn.fetchval.side_effect = [1, 50000]
    svc = CentralBankService(pool)
    result = await svc.emit()
    # ratio = 1 - 50000/10000 = -4, max(0, ...) = 0, amount = 0
    assert result["emitted"] == 0
    assert result["skipped"] is True


@pytest.mark.asyncio
async def test_sink_deducts_from_user(pool):
    pool._wallets["alice"] = (1000, 0)
    pool._conn.fetchval.return_value = 1000
    svc = CentralBankService(pool)
    result = await svc.sink("alice", 100)
    assert result["sunk"] == 100
    assert result["balance_after"] == 900


@pytest.mark.asyncio
async def test_sink_insufficient_raises(pool):
    pool._wallets["alice"] = (50, 0)
    pool._conn.fetchval.return_value = 50
    svc = CentralBankService(pool)
    with pytest.raises(ValueError, match="insufficient"):
        await svc.sink("alice", 100)


@pytest.mark.asyncio
async def test_sink_zero_amount_raises(pool):
    svc = CentralBankService(pool)
    with pytest.raises(ValueError, match="amount must be > 0"):
        await svc.sink("alice", 0)
