from tests.marketplace_e2e_helpers import (
    BUYER_ID,
    CREATOR_ID,
    OCEAN,
    auth_header,
    client,
)


def test_creator_publishes_player_purchases_and_creator_is_paid(client):
    test_client, conn = client

    create = test_client.post(
        "/v1/marketplace/npc-templates",
        json={"name": "Chef Wang", "price_gold": 100, "ocean_json": OCEAN},
        headers=auth_header(CREATOR_ID),
    )
    assert create.status_code == 201
    template_id = create.json()["id"]

    market = test_client.get("/v1/marketplace/npc-templates?status=live")
    assert market.status_code == 200
    assert [item["id"] for item in market.json()] == [template_id]

    purchase = test_client.post(
        "/v1/marketplace/purchase",
        json={
            "template_kind": "npc",
            "template_id": template_id,
            "idempotency_key": "npc-e2e-key",
        },
        headers=auth_header(BUYER_ID),
    )
    assert purchase.status_code == 200

    inventory = test_client.get(
        f"/v1/marketplace/inventory/{BUYER_ID}", headers=auth_header(BUYER_ID)
    )
    assert inventory.status_code == 200
    assert inventory.json() == [
        {
            "purchase_id": purchase.json()["purchase_id"],
            "user_id": BUYER_ID,
            "template_kind": "npc",
            "template_id": template_id,
            "price_paid_gold": 100,
            "created_at": "2026-10-08T00:00:00Z",
        }
    ]

    revenue = test_client.get(
        f"/v1/marketplace/revenue/{CREATOR_ID}", headers=auth_header(CREATOR_ID)
    )
    assert revenue.status_code == 200
    assert revenue.json() == [
        {
            "purchase_id": purchase.json()["purchase_id"],
            "amount_gold": 100,
            "platform_cut_gold": 0,
            "created_at": "2026-10-08T00:00:00Z",
        }
    ]
    assert conn.wallets[BUYER_ID] == 900
    assert conn.wallets[CREATOR_ID] == 100
    assert len(conn.transactions) == 2
