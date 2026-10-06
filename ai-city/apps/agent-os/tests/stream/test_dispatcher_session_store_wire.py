"""Wire SessionStore ↔ dispatcher.say_stream() (C2 follow-up)。

测试目标：
- dispatcher 每个 beat 写入 store
- 优雅退出 → store.mark_done(complete=True)
- 异常退出 → store mark_done(complete=False)
- 不注入 store 时行为兼容（已有 tests/test_dispatcher.py 已覆盖）

不在本测试范围：
- TTL 过期（tests/stream/test_session_store.py 已覆盖）
- 真实 LLM 调用（需要 ANTHROPIC_API_KEY，由 e2e 测试覆盖）
"""
from __future__ import annotations

import asyncio
from unittest.mock import AsyncMock, MagicMock

import pytest

from agent_os.dispatcher import ActionDispatcher
from agent_os.stream.session_store import SessionStore


# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------


@pytest.fixture
def store() -> SessionStore:
    """全新内存 SessionStore（无 mock — 60min TTL）。"""
    return SessionStore()


@pytest.fixture
def wired_dispatcher(store: SessionStore) -> ActionDispatcher:
    """带 mock llm + mock publisher + 真实 store 的 dispatcher。"""
    llm = MagicMock()
    pub = MagicMock()
    pub.publish_beat = AsyncMock()
    pub.publish_done = AsyncMock()
    return ActionDispatcher(llm_client=llm, publisher=pub, session_store=store)


# ---------------------------------------------------------------------------
# Wire 测试
# ---------------------------------------------------------------------------


def test_dispatcher_writes_each_beat_to_session_store(wired_dispatcher, store):
    """say_stream yield 3 events → store buffer 含 2 个 beat。"""

    async def fake_stream(req):
        yield {"text": "<emotion=happy>来了您嘞！</emotion>", "finish_reason": None}
        yield {"text": "<emotion=neutral>几位？</emotion>", "finish_reason": None}
        yield {"text": "<end>", "finish_reason": "stop"}

    wired_dispatcher.llm_client.stream = fake_stream

    async def run():
        events = []
        async for ev in wired_dispatcher.say_stream(
            npc_id="npc_wang_boss_001",
            player_input="点菜",
            npc_context=[],
            session_id="sess-stub",  # caller stub → dispatcher 自动 mint
            trace_id="tr-w",
        ):
            events.append(ev)
        return events

    events = asyncio.run(run())

    # 3 events = 2 beat + 1 done
    assert len(events) == 3

    # dispatcher 自动 mint 了真实 sid（≠ caller 的 stub）
    real_sid = events[0].session_id
    assert real_sid.startswith("sess-")
    assert real_sid != "sess-stub"

    # store buffer 应有 2 个 beat
    buf = store.get_buffer(real_sid)
    assert len(buf) == 2
    assert buf[0] == {"sentence_idx": 0, "text": "来了您嘞！", "emotion": "happy"}
    assert buf[1] == {"sentence_idx": 1, "text": "几位？", "emotion": "neutral"}


def test_dispatcher_marks_done_on_graceful_exit(wired_dispatcher, store):
    """流正常结束 → store.mark_done(complete=True)。"""

    async def fake_stream(req):
        yield {"text": "<emotion=happy>来了您嘞！</emotion>", "finish_reason": None}
        yield {"text": "<end>", "finish_reason": "stop"}

    wired_dispatcher.llm_client.stream = fake_stream

    async def run():
        async for ev in wired_dispatcher.say_stream(
            npc_id="npc_wang_boss_001",
            player_input="点菜",
            npc_context=[],
            session_id="sess-stub",
            trace_id="tr-graceful",
        ):
            pass

    asyncio.run(run())

    # 找到 dispatcher 创建的 sid
    assert len(store._sessions) == 1
    sid = next(iter(store._sessions))
    assert store.is_done(sid) is True
    assert store.is_complete(sid) is True


def test_dispatcher_marks_incomplete_on_error(wired_dispatcher, store):
    """LLM 流异常 → store.mark_done(complete=False)（best-effort + re-raise）。"""

    async def fake_stream(req):
        raise ConnectionError("LLM down")
        yield  # unreachable, makes this an async generator

    wired_dispatcher.llm_client.stream = fake_stream

    async def run():
        async for _ in wired_dispatcher.say_stream(
            npc_id="npc_wang_boss_001",
            player_input="点菜",
            npc_context=[],
            session_id="sess-stub",
            trace_id="tr-err",
        ):
            pass

    with pytest.raises(ConnectionError):
        asyncio.run(run())

    # dispatcher 自动 mint 了 sid → mark_done(complete=False)
    assert len(store._sessions) == 1
    sid = next(iter(store._sessions))
    assert store.is_done(sid) is True
    assert store.is_complete(sid) is False  # 异常退出 → not complete


def test_dispatcher_uses_provided_session_id_if_already_in_store(store):
    """caller 已 create 的 sid → 沿用，不 mint 新 sid。"""

    # caller 先 create（典型：stream 前先拿 sid 给客户端）
    pre_sid = store.create(npc_id="npc_wang_boss_001")

    llm = MagicMock()
    pub = MagicMock()
    pub.publish_beat = AsyncMock()
    pub.publish_done = AsyncMock()
    dispatcher = ActionDispatcher(llm_client=llm, publisher=pub, session_store=store)

    async def fake_stream(req):
        yield {"text": "<emotion=happy>hi</emotion>", "finish_reason": None}
        yield {"text": "<end>", "finish_reason": "stop"}

    dispatcher.llm_client.stream = fake_stream

    async def run():
        events = []
        async for ev in dispatcher.say_stream(
            npc_id="npc_wang_boss_001",
            player_input="hi",
            npc_context=[],
            session_id=pre_sid,
            trace_id="tr-pre",
        ):
            events.append(ev)
        return events

    events = asyncio.run(run())

    # 沿用 caller 的 sid
    assert events[0].session_id == pre_sid
    # 仍只有 1 个 session in store
    assert len(store._sessions) == 1
    assert store.is_complete(pre_sid) is True


def test_dispatcher_no_session_store_incompatible():
    """不注入 session_store 时 → 行为兼容（原 say_stream 测试已验证）。
    此处再次确认 store 参数为 None 时不抛错。
    """
    llm = MagicMock()
    pub = MagicMock()
    pub.publish_beat = AsyncMock()
    pub.publish_done = AsyncMock()
    dispatcher = ActionDispatcher(llm_client=llm, publisher=pub)  # no session_store

    async def fake_stream(req):
        yield {"text": "<emotion=happy>来了您嘞！</emotion>", "finish_reason": None}
        yield {"text": "<emotion=neutral>几位？</emotion>", "finish_reason": None}
        yield {"text": "<end>", "finish_reason": "stop"}

    dispatcher.llm_client.stream = fake_stream

    async def run():
        events = []
        async for ev in dispatcher.say_stream(
            npc_id="npc_wang_boss_001",
            player_input="hi",
            npc_context=[],
            session_id="sess-stub-noop",
            trace_id="tr-noop",
        ):
            events.append(ev)
        return events

    # 不抛错即兼容（store=None 时跳过 append/mark_done）
    events = asyncio.run(run())
    assert len(events) == 3  # 2 beat + 1 done


@pytest.mark.asyncio
async def test_store_append_happens_before_publish_beat():
    """Buffer append must precede Redis publish so a concurrent reconnect can't miss the beat.

    Regression test (review issue #2)：之前 publish_beat 在 store.append 之前，
    重连窗口期 GET /buffer?from_idx=N 漏 beat。现在 store 是 source of truth，
    append 必须先于 publish 完成。
    """
    from unittest.mock import MagicMock

    from agent_os.dispatcher import ActionDispatcher
    from agent_os.stream.session_store import SessionStore

    store = SessionStore()
    call_order: list[str] = []  # 记录 append vs publish_beat 的调用顺序

    # 包一层 store.append 记录顺序（append 是 sync —— 所以 call 时刻 = 完成时刻）
    real_append = store.append

    def tracking_append(sid, sentence_idx, text, emotion):
        call_order.append(f"append:{sid}:{sentence_idx}")
        return real_append(sid, sentence_idx, text, emotion)

    store.append = tracking_append

    # Mock publisher：publish_beat 故意慢 10ms，给 event loop 插队的窗口
    publisher = MagicMock()
    publisher.publish_done = AsyncMock()

    async def slow_publish_beat(**kwargs):
        call_order.append(
            f"publish:beat:{kwargs['session_id']}:{kwargs['sentence_idx']}"
        )
        await asyncio.sleep(0.01)

    publisher.publish_beat = slow_publish_beat

    # LLM 流：2 个 beat + end
    llm = MagicMock()

    async def fake_stream(req):
        yield {"text": "<emotion=happy>来了您嘞！</emotion>", "finish_reason": None}
        yield {"text": "<emotion=neutral>几位？</emotion>", "finish_reason": None}
        yield {"text": "<end>", "finish_reason": "stop"}

    llm.stream = fake_stream

    dispatcher = ActionDispatcher(
        llm_client=llm, publisher=publisher, session_store=store
    )

    events = []
    async for ev in dispatcher.say_stream(
        npc_id="npc_wang_boss_001",
        player_input="hi",
        npc_context=[],
        session_id="sess-stub",
        trace_id="tr-ordering",
    ):
        events.append(ev)

    # dispatcher mint 的 sid
    real_sid = events[0].session_id

    # 核心断言 —— append:N 必须在 publish:beat:N 之前
    assert call_order == [
        f"append:{real_sid}:0",
        f"publish:beat:{real_sid}:0",
        f"append:{real_sid}:1",
        f"publish:beat:{real_sid}:1",
    ], f"Expected append-then-publish ordering, got: {call_order}"

    # buffer 里有 2 个 beat（即使 publish_beat 慢，append 已经完成）
    assert len(store.get_buffer(real_sid)) == 2