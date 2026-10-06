"""Stage 3 / C2 follow-up：HTTP 重连补帧端点 GET /v1/npc/sessions/{sid}/buffer。

测试覆盖：
- 已知 session → 返 beats + complete 标志
- from_idx 切片正确
- 未知 session → 400 R_015
- done session → complete=True
- 进行中 session → complete=False
- from_idx 越界 → 空 beats 列表
"""
from __future__ import annotations

import asyncio
from pathlib import Path
from unittest.mock import AsyncMock, MagicMock

import pytest
from fastapi.testclient import TestClient

from agent_os.stream.session_store import SessionStore


# ---------------------------------------------------------------------------
# Fixtures: minimal FastAPI app + 注入已知 sid 的 store
# ---------------------------------------------------------------------------


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


@pytest.fixture
def client_with_store(monkeypatch, templates_dir: Path):
    """起一个真实 FastAPI app，把 app.state.session_store 换成 fresh SessionStore
    并预填 5 个 beat。返回 (TestClient, SessionStore, sid)。
    """
    monkeypatch.setenv("HTTP_PORT", "8084")
    monkeypatch.setenv("REDIS_URL", "redis://127.0.0.1:6379/0")
    monkeypatch.setenv("NPC_TEMPLATES_DIR", str(templates_dir))
    monkeypatch.setenv("SAY_TICK_SECONDS", "0.05")

    import importlib

    from agent_os import app as app_module

    importlib.reload(app_module)
    from agent_os.app import create_app

    app = create_app()

    # 替换 store 为 fresh 实例 + 预填数据
    fresh_store = SessionStore()
    sid = fresh_store.create(npc_id="npc_wang_boss_001")
    for i in range(5):
        fresh_store.append(
            sid, sentence_idx=i, text=f"句{i}", emotion="neutral"
        )

    app.state.session_store = fresh_store

    with TestClient(app) as client:
        yield client, fresh_store, sid


# ---------------------------------------------------------------------------
# 端点测试
# ---------------------------------------------------------------------------


def test_endpoint_returns_beats_from_index(client_with_store):
    """5 个 beat 预填，from_idx=2 应返 3 个。"""
    client, _store, sid = client_with_store

    resp = client.get(f"/v1/npc/sessions/{sid}/buffer?from_idx=2")
    assert resp.status_code == 200
    body = resp.json()
    assert body["session_id"] == sid
    assert len(body["beats"]) == 3
    assert body["beats"][0]["sentence_idx"] == 2
    assert body["beats"][1]["sentence_idx"] == 3
    assert body["beats"][2]["sentence_idx"] == 4
    assert body["sentence_count"] == 5  # 包含 from_idx 之前的


def test_endpoint_default_from_idx_returns_all(client_with_store):
    """from_idx 缺省 = 0 → 返全部 5 个 beat。"""
    client, _store, sid = client_with_store

    resp = client.get(f"/v1/npc/sessions/{sid}/buffer")
    assert resp.status_code == 200
    body = resp.json()
    assert len(body["beats"]) == 5
    assert body["sentence_count"] == 5


def test_endpoint_returns_400_on_unknown_session(client_with_store):
    """不存在的 sid → 400 + R_015 detail。"""
    client, _store, _sid = client_with_store

    resp = client.get("/v1/npc/sessions/sess-unknown/buffer")
    assert resp.status_code == 400
    body = resp.json()
    assert body["detail"]["code"] == "R_015"
    assert "session_id" in body["detail"]


def test_endpoint_marks_done_includes_complete_flag(client_with_store):
    """mark_done(complete=True) 后 → complete=True；未调用 → complete=False。"""
    client, store, sid = client_with_store

    # 调用 mark_done(complete=True)
    store.mark_done(sid, complete=True)
    resp = client.get(f"/v1/npc/sessions/{sid}/buffer")
    assert resp.status_code == 200
    assert resp.json()["complete"] is True

    # 新 session（未 mark_done）→ complete=False
    sid2 = store.create(npc_id="npc_test_002")
    resp2 = client.get(f"/v1/npc/sessions/{sid2}/buffer")
    assert resp2.status_code == 200
    assert resp2.json()["complete"] is False


def test_endpoint_incomplete_stream_complete_false(client_with_store):
    """流异常退出（mark_done(complete=False)）→ complete=False。"""
    client, store, sid = client_with_store

    store.mark_done(sid, complete=False)
    resp = client.get(f"/v1/npc/sessions/{sid}/buffer")
    assert resp.status_code == 200
    assert resp.json()["complete"] is False


def test_endpoint_from_idx_out_of_range_returns_empty_beats(client_with_store):
    """from_idx 越界 → 空 beats + 完整 sentence_count。"""
    client, _store, sid = client_with_store

    resp = client.get(f"/v1/npc/sessions/{sid}/buffer?from_idx=10")
    assert resp.status_code == 200
    body = resp.json()
    assert body["beats"] == []
    assert body["sentence_count"] == 5  # store 里总数仍返


def test_endpoint_negative_from_idx_rejected(client_with_store):
    """from_idx < 0 → 422 (Query ge=0 校验)。"""
    client, _store, sid = client_with_store

    resp = client.get(f"/v1/npc/sessions/{sid}/buffer?from_idx=-1")
    assert resp.status_code == 422