"""Settings-driven channel routing (T12 C1 follow-up).

确认 Publisher 真的在调用时读取 ``REDIS_CHANNEL_NPC_SAY_STREAM`` env，而不是
沿用模块级 import-time 常量 —— 否则 docker-compose 里设的值就是 no-op。
"""
from __future__ import annotations

import asyncio
import os
from unittest.mock import AsyncMock, MagicMock

import pytest

from agent_os.settings import get_redis_channel_npc_say_stream
from agent_os.stream.publisher import Publisher


def test_default_channel_is_npc_say_stream(monkeypatch: pytest.MonkeyPatch) -> None:
    """未设 env → 走默认值（保持向后兼容）。"""
    monkeypatch.delenv("REDIS_CHANNEL_NPC_SAY_STREAM", raising=False)
    assert get_redis_channel_npc_say_stream() == "aicity:npc:say_stream"


def test_empty_env_falls_back_to_default(monkeypatch: pytest.MonkeyPatch) -> None:
    """env 设成空串 → 走默认值（os.getenv 不替代 default）。"""
    monkeypatch.setenv("REDIS_CHANNEL_NPC_SAY_STREAM", "")
    assert get_redis_channel_npc_say_stream() == "aicity:npc:say_stream"


def test_whitespace_env_falls_back_to_default(monkeypatch: pytest.MonkeyPatch) -> None:
    """env 设成纯空白 → 走默认值（避免 redis.publish("   ") 静默失败）。"""
    monkeypatch.setenv("REDIS_CHANNEL_NPC_SAY_STREAM", "   ")
    assert get_redis_channel_npc_say_stream() == "aicity:npc:say_stream"


def test_env_override(monkeypatch: pytest.MonkeyPatch) -> None:
    """设了 env → 走自定义值。"""
    monkeypatch.setenv("REDIS_CHANNEL_NPC_SAY_STREAM", "test:custom:channel")
    assert get_redis_channel_npc_say_stream() == "test:custom:channel"


def test_publish_uses_env_override_at_call_time(monkeypatch: pytest.MonkeyPatch) -> None:
    """publish_beat 调到 redis 时用的是 env 当前值，不是 import-time 常量。"""
    mock_redis = MagicMock()
    mock_redis.publish = AsyncMock(return_value=1)

    monkeypatch.setenv("REDIS_CHANNEL_NPC_SAY_STREAM", "test:env:channel")
    pub = Publisher(redis=mock_redis)

    async def run() -> None:
        await pub.publish_beat(
            npc_id="npc_a",
            session_id="s1",
            sentence_idx=0,
            text="hi",
            emotion="happy",
            trace_id="t1",
        )

    asyncio.run(run())

    mock_redis.publish.assert_called()
    channel_arg = mock_redis.publish.call_args[0][0]
    assert channel_arg == "test:env:channel"


def test_publish_done_uses_env_override(monkeypatch: pytest.MonkeyPatch) -> None:
    """publish_done 也走动态 channel。"""
    mock_redis = MagicMock()
    mock_redis.publish = AsyncMock(return_value=1)

    monkeypatch.setenv("REDIS_CHANNEL_NPC_SAY_STREAM", "stage2:override:done")
    pub = Publisher(redis=mock_redis)

    async def run() -> None:
        await pub.publish_done(
            npc_id="npc_a",
            session_id="s1",
            sentence_count=1,
            complete=True,
            trace_id="t1",
        )

    asyncio.run(run())

    channel_arg = mock_redis.publish.call_args[0][0]
    assert channel_arg == "stage2:override:done"


def test_env_override_can_be_changed_between_calls(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """两次 publish 之间切换 env → 各自走对应 channel（证明每次都重读）。"""
    mock_redis = MagicMock()
    mock_redis.publish = AsyncMock(return_value=1)

    pub = Publisher(redis=mock_redis)

    async def run() -> None:
        monkeypatch.setenv("REDIS_CHANNEL_NPC_SAY_STREAM", "channel:A")
        await pub.publish_beat(
            npc_id="n1", session_id="s", sentence_idx=0,
            text="a", emotion="e", trace_id="t",
        )
        monkeypatch.setenv("REDIS_CHANNEL_NPC_SAY_STREAM", "channel:B")
        await pub.publish_done(
            npc_id="n1", session_id="s", sentence_count=1,
            complete=True, trace_id="t",
        )

    asyncio.run(run())

    channels = [c.args[0] for c in mock_redis.publish.call_args_list]
    assert channels == ["channel:A", "channel:B"]