"""SessionStore：60min TTL 内存 session map + 重连补帧 buffer。"""
import time
import pytest
from agent_os.stream.session_store import SessionStore


def test_create_session():
    s = SessionStore()
    sid = s.create(npc_id="npc_wang_boss_001")
    assert sid.startswith("sess-")
    assert len(sid) == len("sess-") + 12


def test_get_session_returns_buffer():
    s = SessionStore()
    sid = s.create(npc_id="npc_wang_boss_001")
    s.append(sid, sentence_idx=0, text="来了您嘞！", emotion="happy")
    s.append(sid, sentence_idx=1, text="几位？", emotion="neutral")
    buf = s.get_buffer(sid)
    assert buf == [
        {"sentence_idx": 0, "text": "来了您嘞！", "emotion": "happy"},
        {"sentence_idx": 1, "text": "几位？", "emotion": "neutral"},
    ]


def test_get_buffer_from_idx():
    s = SessionStore()
    sid = s.create(npc_id="npc_wang_boss_001")
    for i in range(3):
        s.append(sid, sentence_idx=i, text=f"句{i}", emotion="neutral")
    buf = s.get_buffer(sid, from_idx=2)
    assert len(buf) == 1
    assert buf[0]["sentence_idx"] == 2


def test_get_unknown_session_raises():
    from agent_os.errors import R015SessionNotFound
    s = SessionStore()
    with pytest.raises(R015SessionNotFound):
        s.get_buffer("sess-unknown")


def test_ttl_eviction(monkeypatch):
    """60min 过期 → session 自动失效。"""
    s = SessionStore(ttl_seconds=60)
    sid = s.create(npc_id="npc_wang_boss_001")
    # 模拟时间过去 61 分钟 — 捕获原始 time.time 避免 lambda 递归
    real_time = time.time
    monkeypatch.setattr(time, "time", lambda: real_time() + 61 * 60)
    from agent_os.errors import R015SessionNotFound
    with pytest.raises(R015SessionNotFound):
        s.get_buffer(sid)
