import json
from unittest.mock import AsyncMock, MagicMock

import pytest
from economy_service.services.marketplace_service import MarketplaceService

OCEAN = {"O": 0.7, "C": 0.8, "E": 0.5, "A": 0.6, "N": 0.3}


@pytest.fixture
def pool():
    conn = AsyncMock()
    conn.fetchrow.return_value = {"id": 11}
    conn.fetch.return_value = [
        {"id": 12, "status": "live"},
        {"id": 13, "status": "taken_down"},
    ]
    pool = MagicMock()
    pool.acquire.return_value.__aenter__ = AsyncMock(return_value=conn)
    pool.acquire.return_value.__aexit__ = AsyncMock(return_value=False)
    pool._conn = conn
    return pool


@pytest.mark.asyncio
async def test_create_npc_template_serializes_json_columns(pool):
    service = MarketplaceService(pool)

    template_id = await service.create_npc_template(
        creator_id="00000000-0000-0000-0000-000000000001",
        name="Chef Wang",
        ocean_json=OCEAN,
        price_gold=100,
        bt_skeleton='{"type":"action","name":"cook"}',
        product_catalog=[{"name": "dumpling", "price_gold": 3}],
    )

    assert template_id == 11
    sql, *params = pool._conn.fetchrow.call_args.args
    assert "INSERT INTO npc_template" in sql
    assert params[0] == "00000000-0000-0000-0000-000000000001"
    assert params[3] == json.dumps(OCEAN)
    assert params[5] == json.dumps([{"name": "dumpling", "price_gold": 3}])


@pytest.mark.asyncio
async def test_list_npc_templates_filters_by_status(pool):
    service = MarketplaceService(pool)

    items = await service.list_npc_templates(status="live", limit=10, offset=5)

    assert [item["id"] for item in items] == [12, 13]
    sql, *params = pool._conn.fetch.call_args.args
    assert "WHERE status = $1" in sql
    assert tuple(params) == ("live", 10, 5)


@pytest.mark.asyncio
async def test_get_npc_template_returns_dict(pool):
    pool._conn.fetchrow.return_value = {"id": 11, "name": "Chef Wang"}
    service = MarketplaceService(pool)

    item = await service.get_npc_template(11)

    assert item == {"id": 11, "name": "Chef Wang"}


@pytest.mark.asyncio
async def test_take_down_npc_template_updates_status(pool):
    service = MarketplaceService(pool)

    await service.take_down_npc_template(11, actor_id="00000000-0000-0000-0000-000000000002")

    sql, *params = pool._conn.execute.call_args.args
    assert "UPDATE npc_template" in sql
    assert "SET status = 'taken_down'" in sql
    assert tuple(params) == (11,)
