from unittest.mock import AsyncMock, MagicMock

import pytest
from economy_service.services.marketplace_service import MarketplaceService

YAML_CONTENT = "saga:\n  name: welcome\n"


@pytest.fixture
def pool():
    conn = AsyncMock()
    conn.fetchrow.return_value = {"id": 31}
    conn.fetch.return_value = [
        {"id": 32, "status": "live"},
        {"id": 33, "status": "taken_down"},
    ]
    pool = MagicMock()
    pool.acquire.return_value.__aenter__ = AsyncMock(return_value=conn)
    pool.acquire.return_value.__aexit__ = AsyncMock(return_value=False)
    pool._conn = conn
    return pool


@pytest.mark.asyncio
async def test_create_saga_template_serializes_npc_deps(pool):
    service = MarketplaceService(pool)

    template_id = await service.create_saga_template(
        creator_id="00000000-0000-0000-0000-000000000003",
        name="Welcome Saga",
        price_gold=100,
        yaml_content=YAML_CONTENT,
        semantic_version="1.0.0",
        npc_deps=["npc_wang_boss_001"],
        icon_url="https://example.test/icon.png",
        description="A demo saga",
    )

    assert template_id == 31
    sql, *params = pool._conn.fetchrow.call_args.args
    assert "INSERT INTO saga_template" in sql
    assert params[0] == "00000000-0000-0000-0000-000000000003"
    assert params[2] == 100
    assert params[6] == ["npc_wang_boss_001"]
    assert params[7] == "1.0.0"


@pytest.mark.asyncio
async def test_list_saga_templates_filters_by_status(pool):
    service = MarketplaceService(pool)

    items = await service.list_saga_templates(status="live", limit=10, offset=5)

    assert [item["id"] for item in items] == [32, 33]
    sql, *params = pool._conn.fetch.call_args.args
    assert "WHERE status = $1" in sql
    assert tuple(params) == ("live", 10, 5)


@pytest.mark.asyncio
async def test_get_saga_template_returns_dict(pool):
    pool._conn.fetchrow.return_value = {"id": 31, "name": "Welcome Saga"}
    service = MarketplaceService(pool)

    item = await service.get_saga_template(31)

    assert item == {"id": 31, "name": "Welcome Saga"}


@pytest.mark.asyncio
async def test_take_down_saga_template_updates_status(pool):
    service = MarketplaceService(pool)

    await service.take_down_saga_template(31, admin_id="00000000-0000-0000-0000-000000000002")

    sql, *params = pool._conn.execute.call_args.args
    assert "UPDATE saga_template" in sql
    assert "SET status = 'taken_down'" in sql
    assert tuple(params) == (31,)
