import pytest
from memory_service.main import app
from fastapi.testclient import TestClient

client = TestClient(app)

def test_recall_endpoint():
    resp = client.post("/v1/recall", json={
        "npc_id": "npc_a_wang_boss",
        "player_id": "demo_a",
        "query": "刚才的红烧肉",
        "top_k": 5,
    })
    assert resp.status_code == 200
    data = resp.json()
    assert "messages" in data
    assert "scores" in data
