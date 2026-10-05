"""Publisher: 节拍包 → Redis aicity:npc:say_stream 频道 + 100 条 buffer 重试。"""
import asyncio
import json
from unittest.mock import AsyncMock, MagicMock
import pytest

from agent_os.stream.publisher import Publisher


@pytest.fixture
def mock_redis():
    r = MagicMock()
    r.publish = AsyncMock(return_value=1)
    return r


def test_publish_beat_payload_format(mock_redis):
    """节拍包 JSON 结构验证。"""
    pub = Publisher(redis=mock_redis)

    async def run():
        await pub.publish_beat(
            npc_id="npc_wang_boss_001",
            session_id="sess-abc123",
            sentence_idx=0,
            text="来了您嘞！",
            emotion="happy",
            trace_id="tr-abc123def456",
        )

    asyncio.run(run())
    call_args = mock_redis.publish.call_args
    channel = call_args[0][0]
    payload = json.loads(call_args[0][1])

    assert channel == "aicity:npc:say_stream"
    assert payload["type"] == "npc_say_stream"
    assert payload["npc_id"] == "npc_wang_boss_001"
    assert payload["session_id"] == "sess-abc123"
    assert payload["sentence_idx"] == 0
    assert payload["text"] == "来了您嘞！"
    assert payload["emotion"] == "happy"
    assert payload["trace_id"] == "tr-abc123def456"
    assert "ts_ms" in payload


def test_publish_done_payload(mock_redis):
    pub = Publisher(redis=mock_redis)

    async def run():
        await pub.publish_done(
            npc_id="npc_wang_boss_001",
            session_id="sess-abc123",
            sentence_count=3,
            complete=True,
            trace_id="tr-abc123def456",
        )

    asyncio.run(run())
    call_args = mock_redis.publish.call_args
    channel = call_args[0][0]
    payload = json.loads(call_args[0][1])

    assert channel == "aicity:npc:say_stream"
    assert payload["type"] == "npc_say_stream_done"
    assert payload["sentence_count"] == 3
    assert payload["complete"] is True


def test_publish_failure_buffered(mock_redis):
    """Redis 失败 → 内存 buffer 100 条 + 异步重试。"""
    mock_redis.publish = AsyncMock(side_effect=ConnectionError("redis down"))
    pub = Publisher(redis=mock_redis, max_buffer=100)

    async def run():
        await pub.publish_beat(
            npc_id="npc_wang_boss_001",
            session_id="sess-abc123",
            sentence_idx=0,
            text="test",
            emotion="neutral",
            trace_id="tr-abc",
        )

    with pytest.raises(ConnectionError):
        asyncio.run(run())
    assert len(pub._buffer) == 1