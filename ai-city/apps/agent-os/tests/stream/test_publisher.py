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


def test_buffer_capped_at_max(mock_redis):
    """100+ failures → buffer capped at max_buffer (no off-by-one overflow)."""
    mock_redis.publish = AsyncMock(side_effect=ConnectionError("down"))
    pub = Publisher(redis=mock_redis, max_buffer=5, retry_intervals=(0.01, 0.01, 0.01))

    async def run():
        for i in range(20):
            with pytest.raises(ConnectionError):
                await pub.publish_beat(
                    npc_id="n", session_id="s", sentence_idx=i,
                    text="t", emotion="e", trace_id="tr",
                )

    asyncio.run(run())
    assert len(pub._buffer) == 5  # not 6, not 20


def test_retry_succeeds_on_third_attempt(mock_redis):
    """Fail×2 then succeed → no buffer, no raise; publish called 3 times."""
    mock_redis.publish = AsyncMock(side_effect=[
        ConnectionError("x"), ConnectionError("y"), 1,  # success on 3rd
    ])
    pub = Publisher(redis=mock_redis, retry_intervals=(0.01, 0.01, 0.01))

    async def run():
        await pub.publish_beat(
            npc_id="n", session_id="s", sentence_idx=0,
            text="t", emotion="e", trace_id="tr",
        )

    asyncio.run(run())  # must NOT raise
    assert mock_redis.publish.await_count == 3
    assert pub._buffer == []


def test_flush_buffer_drains_on_subsequent_success(mock_redis):
    """第一次失败入 buffer，第二次成功时 drain buffer 中的 payload。"""
    # First publish_beat: 4 retry attempts all fail → 1 payload in buffer + raise
    # Second publish_beat: succeeds on first attempt → triggers _flush_buffer
    # _flush_buffer (async) retries the buffered payload → succeeds
    publish_responses = [
        ConnectionError("1st try fail"),
        ConnectionError("2nd try fail"),
        ConnectionError("3rd try fail"),
        ConnectionError("4th try fail"),
        1,  # second publish_beat's first attempt succeeds
        1,  # _flush_buffer's async drain succeeds
    ]
    mock_redis.publish = AsyncMock(side_effect=publish_responses)
    pub = Publisher(redis=mock_redis, retry_intervals=(0.01, 0.01, 0.01))

    async def run():
        # First publish_beat — exhausts all 4 retries, buffers payload, raises
        with pytest.raises(ConnectionError):
            await pub.publish_beat(
                npc_id="n", session_id="s", sentence_idx=0,
                text="t", emotion="e", trace_id="tr",
            )
        assert len(pub._buffer) == 1

        # Second publish_beat — succeeds, triggers _flush_buffer (async)
        await pub.publish_beat(
            npc_id="n", session_id="s", sentence_idx=1,
            text="t2", emotion="e", trace_id="tr",
        )

        # Give the fire-and-forget _flush task a chance to run
        await asyncio.sleep(0.05)

    asyncio.run(run())
    # After flush: buffer should be drained (back to 0)
    assert len(pub._buffer) == 0
    # mock_redis.publish called 4 (first failed retries) + 1 (second success) + 1 (flush) = 6 times
    assert mock_redis.publish.await_count == 6