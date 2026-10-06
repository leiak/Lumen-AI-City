"""Unit tests for kafka_producer + redis_client."""
from __future__ import annotations
import asyncio
import pytest
from unittest.mock import AsyncMock, MagicMock

from economy_service.clients.kafka_producer import KafkaProducer
from economy_service.clients.redis_client import RedisClient


@pytest.mark.asyncio
async def test_kafka_producer_send_without_producer_logs_and_returns():
    """send() with no producer started → logs debug, no exception."""
    p = KafkaProducer(bootstrap="nonexistent:9092")
    # Don't call start() → _producer remains None
    await p.send("econ.tx.completed", {"tx_id": 1, "amount": 100})


@pytest.mark.asyncio
async def test_kafka_producer_send_calls_underlying():
    """send() schedules background task that invokes _producer.send_and_wait."""
    p = KafkaProducer()
    p._producer = MagicMock()
    p._producer.send_and_wait = AsyncMock()
    await p.send("econ.tx.completed", {"tx_id": 42, "amount": 100})
    # Drain scheduled tasks
    await asyncio.sleep(0)
    p._producer.send_and_wait.assert_awaited_once()
    args, _ = p._producer.send_and_wait.call_args
    assert args[0] == "econ.tx.completed"


@pytest.mark.asyncio
async def test_kafka_producer_send_returns_immediately_without_blocking():
    """send() is fire-and-forget: caller returns before broker roundtrip."""
    p = KafkaProducer()
    # Make send_and_wait sleep 1s to prove send() does not block on it
    p._producer = MagicMock()

    async def slow_send(topic, payload, timeout=2.0):
        await asyncio.sleep(1.0)

    p._producer.send_and_wait = AsyncMock(side_effect=slow_send)

    import time
    t0 = time.monotonic()
    await p.send("econ.tx.completed", {"tx_id": 1})
    elapsed = time.monotonic() - t0
    # Caller should return < 100ms — proves not blocked by 1s broker roundtrip
    assert elapsed < 0.1, f"send() blocked for {elapsed:.3f}s (expected < 0.1s)"


@pytest.mark.asyncio
async def test_kafka_producer_send_logs_on_background_failure():
    """_send_one() catches exception → logs warning, doesn't propagate."""
    p = KafkaProducer()
    p._producer = MagicMock()
    p._producer.send_and_wait = AsyncMock(side_effect=RuntimeError("broker down"))
    # Call _send_one directly (background task context)
    await p._send_one("econ.tx.completed", {"tx_id": 1})
    # If we get here without exception → exception was swallowed correctly
    p._producer.send_and_wait.assert_awaited_once()


@pytest.mark.asyncio
async def test_redis_client_cache_balance_writes_both_keys():
    """cache_balance(gold=X, token=Y) sets both keys."""
    r = RedisClient(url="redis://localhost:6379/0")
    r._redis = MagicMock()
    r._redis.set = AsyncMock()
    await r.cache_balance("alice", gold=100, token=50)
    assert r._redis.set.await_count == 2


@pytest.mark.asyncio
async def test_redis_client_cache_balance_partial_update_gold_only():
    """cache_balance(gold=X) only writes gold key — token untouched."""
    r = RedisClient(url="redis://localhost:6379/0")
    r._redis = MagicMock()
    r._redis.set = AsyncMock()
    await r.cache_balance("alice", gold=100)
    # Only 1 SET (token not touched — prevents overwriting valid cached token=500 with 0)
    assert r._redis.set.await_count == 1
    args, _ = r._redis.set.call_args
    assert "gold" in args[0]
    assert "token" not in args[0]


@pytest.mark.asyncio
async def test_redis_client_cache_balance_partial_update_token_only():
    """cache_balance(token=Y) only writes token key — gold untouched."""
    r = RedisClient(url="redis://localhost:6379/0")
    r._redis = MagicMock()
    r._redis.set = AsyncMock()
    await r.cache_balance("alice", token=50)
    assert r._redis.set.await_count == 1
    args, _ = r._redis.set.call_args
    assert "token" in args[0]
    assert "gold" not in args[0]


@pytest.mark.asyncio
async def test_redis_client_cache_balance_no_kwargs_is_noop():
    """cache_balance() with no kwargs → no SETs (prevents accidental overwrite)."""
    r = RedisClient(url="redis://localhost:6379/0")
    r._redis = MagicMock()
    r._redis.set = AsyncMock()
    await r.cache_balance("alice")
    assert r._redis.set.await_count == 0


@pytest.mark.asyncio
async def test_redis_client_get_cached_returns_none_when_no_keys():
    """get_cached_balance → None when both keys are absent."""
    r = RedisClient()
    r._redis = MagicMock()
    r._redis.get = AsyncMock(return_value=None)
    result = await r.get_cached_balance("alice")
    assert result is None