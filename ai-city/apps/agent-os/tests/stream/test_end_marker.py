"""EndMarker：<end> tag / 3s 超时 → npc_say_stream_done 包生成。"""
import time
from agent_os.stream.end_marker import EndMarker


def test_explicit_end_marker():
    m = EndMarker(timeout_seconds=3.0)
    assert m.feed("<end>") is True
    assert m.done is True


def test_timeout_triggers_implicit_done(monkeypatch):
    """3s 无新 token → 隐式 done (complete=False)."""
    import time as time_module
    m = EndMarker(timeout_seconds=3.0)
    m.feed("partial sentence without end")
    # 模拟时间过去 4s — 捕获原始 time.time 避免 lambda 递归
    real_time = time.time
    monkeypatch.setattr(time_module, "time", lambda: real_time() + 4.0)
    assert m.check_timeout() is True
    assert m.done is True
    assert m.complete is False  # 隐式 done → complete=False


def test_no_end_no_timeout():
    m = EndMarker(timeout_seconds=3.0)
    assert m.feed("partial sentence without end") is False
    assert m.done is False


def test_complete_flag():
    m = EndMarker(timeout_seconds=3.0)
    m.feed("<end>")
    assert m.complete is True  # 显式 end → complete=True


def test_initial_state():
    """新创建的 EndMarker：done=False, complete=False。"""
    m = EndMarker()
    assert m.done is False
    assert m.complete is False


def test_feed_after_done_is_idempotent():
    """done=True 后再 feed：返回 False，不重置 done/complete。"""
    m = EndMarker()
    m.feed("<end>")
    assert m.feed("more text") is False
    assert m.done is True
    assert m.complete is True


def test_check_timeout_after_done_returns_false():
    """done=True 后 check_timeout：返回 False（不复活，不破坏状态）。"""
    m = EndMarker()
    m.feed("<end>")
    assert m.check_timeout() is False
    assert m.done is True
    assert m.complete is True