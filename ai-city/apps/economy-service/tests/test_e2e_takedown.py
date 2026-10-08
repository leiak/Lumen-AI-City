from tests.marketplace_e2e_helpers import (
    ADMIN_ID,
    BUYER_ID,
    CREATOR_ID,
    OCEAN,
    auth_header,
    client,
)


def test_takedown_hides_live_market_but_keeps_previous_inventory(client):
    test_client, _ = client

    create = test_client.post(
        "/v1/marketplace/npc-templates",
        json={"name": "Chef Wang", "price_gold": 100, "ocean_json": OCEAN},
        headers=auth_header(CREATOR_ID),
    )
    assert create.status_code == 201
    template_id = create.json()["id"]

    purchase = test_client.post(
        "/v1/marketplace/purchase",
        json={
            "template_kind": "npc",
            "template_id": template_id,
            "idempotency_key": "before-takedown",
        },
        headers=auth_header(BUYER_ID),
    )
    assert purchase.status_code == 200

    take_down = test_client.post(
        f"/v1/marketplace/npc-templates/{template_id}/take-down",
        headers=auth_header(ADMIN_ID),
    )
    assert take_down.status_code == 200

    market = test_client.get("/v1/marketplace/npc-templates?status=live")
    assert market.status_code == 200
    assert market.json() == []

    denied = test_client.post(
        "/v1/marketplace/purchase",
        json={
            "template_kind": "npc",
            "template_id": template_id,
            "idempotency_key": "after-takedown",
        },
        headers=auth_header(BUYER_ID),
    )
    assert denied.status_code == 410

    inventory = test_client.get(
        f"/v1/marketplace/inventory/{BUYER_ID}", headers=auth_header(BUYER_ID)
    )
    assert inventory.status_code == 200
    assert len(inventory.json()) == 1
