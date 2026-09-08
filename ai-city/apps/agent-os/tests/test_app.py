"""FastAPI app lifespan + healthz."""
from pathlib import Path

import pytest
from fastapi.testclient import TestClient


@pytest.fixture
def templates_dir(tmp_path: Path) -> Path:
    f = tmp_path / "wang.yaml"
    f.write_text(
        "npc_id: npc_wang_boss_001\n"
        "enabled: true\n"
        "say:\n"
        "  greeting:\n"
        "    - '来了您嘞！'\n",
        encoding="utf-8",
    )
    return tmp_path


def test_app_starts_and_healthz_returns_ok(monkeypatch, templates_dir: Path):
    """Config from env, lifespan wires redis pub + scheduler."""
    monkeypatch.setenv("HTTP_PORT", "8084")
    monkeypatch.setenv("REDIS_URL", "redis://127.0.0.1:6379/0")
    monkeypatch.setenv("NPC_TEMPLATES_DIR", str(templates_dir))
    monkeypatch.setenv("SAY_TICK_SECONDS", "0.05")

    # Must import after env set since config loads at import
    import importlib

    from agent_os import app as app_module

    importlib.reload(app_module)
    from agent_os.app import create_app

    app = create_app()
    with TestClient(app) as client:
        resp = client.get("/healthz")
        assert resp.status_code == 200
        assert resp.json() == {"status": "ok"}
