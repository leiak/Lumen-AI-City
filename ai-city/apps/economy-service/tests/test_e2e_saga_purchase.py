from tests.marketplace_e2e_helpers import (
    BUYER_ID,
    CREATOR_ID,
    NPC_YAML,
    auth_header,
    client,
)


def test_saga_with_npc_deps_can_be_published_and_purchased(client):
    test_client, _ = client

    create = test_client.post(
        "/v1/marketplace/saga-templates",
        json={
            "name": "Welcome Saga",
            "price_gold": 100,
            "yaml_content": NPC_YAML,
            "semantic_version": "1.0.0",
            "npc_deps": ["npc_wang_boss_001"],
        },
        headers=auth_header(CREATOR_ID),
    )
    assert create.status_code == 201
    template_id = create.json()["id"]

    market = test_client.get("/v1/marketplace/saga-templates?status=live")
    assert market.status_code == 200
    assert [item["id"] for item in market.json()] == [template_id]

    detail = test_client.get(f"/v1/marketplace/saga-templates/{template_id}")
    assert detail.status_code == 200
    assert detail.json()["npc_deps"] == ["npc_wang_boss_001"]

    purchase = test_client.post(
        "/v1/marketplace/purchase",
        json={
            "template_kind": "saga",
            "template_id": template_id,
            "idempotency_key": "saga-e2e-key",
        },
        headers=auth_header(BUYER_ID),
    )
    assert purchase.status_code == 200

    inventory = test_client.get(
        f"/v1/marketplace/inventory/{BUYER_ID}", headers=auth_header(BUYER_ID)
    )
    assert inventory.status_code == 200
    assert inventory.json()[0]["template_kind"] == "saga"
