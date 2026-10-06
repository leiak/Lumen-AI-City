import pytest
from economy_service.services.wallet_service import WalletService


@pytest.mark.asyncio
async def test_get_wallet_returns_row(pool):
    pool._wallets["alice"] = (1000, 0)
    svc = WalletService(pool)
    w = await svc.get_wallet("alice")
    assert w["gold_balance"] == 1000


@pytest.mark.asyncio
async def test_get_wallet_returns_none_when_missing(pool):
    svc = WalletService(pool)
    assert await svc.get_wallet("bob") is None
