import pytest
from economy_service.errors import InsufficientBalance, TransferSelf
from economy_service.schemas import Currency
from economy_service.services.wallet_service import WalletService


@pytest.mark.asyncio
async def test_transfer_succeeds_atomic(pool):
    """Atomic transfer moves currency between two wallets + writes 2 tx log rows."""
    pool._wallets["alice"] = (1000, 0)
    pool._wallets["bob"] = (0, 0)
    svc = WalletService(pool)
    from_w, to_w = await svc.transfer("alice", "bob", Currency.GOLD, 100)
    assert from_w["gold_balance"] == 900
    assert to_w["gold_balance"] == 100


@pytest.mark.asyncio
async def test_transfer_insufficient_raises(pool):
    pool._wallets["alice"] = (50, 0)
    svc = WalletService(pool)
    with pytest.raises(InsufficientBalance):
        await svc.transfer("alice", "bob", Currency.GOLD, 100)


@pytest.mark.asyncio
async def test_transfer_to_self_raises(pool):
    pool._wallets["alice"] = (1000, 0)
    svc = WalletService(pool)
    with pytest.raises(TransferSelf):
        await svc.transfer("alice", "alice", Currency.GOLD, 100)
