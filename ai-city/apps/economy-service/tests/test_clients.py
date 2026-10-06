"""Unit tests for kafka_producer + redis_client."""
from __future__ import annotations
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
    """send() invokes _producer.send_and_wait with correct args."""
    p = KafkaProducer()
    p._producer = MagicMock()
    p._producer.send_and_wait = AsyncMock()
    await p.send("econ.tx.completed", {"tx_id": 42, "amount": 100})
    p._producer.send_and_wait.assert_awaited_once()
    args, _ = p._producer.send_and_wait.call_args
    assert args[0] == "econ.tx.completed"


@pytest.mark.asyncio
async def test_redis_client_cache_balance_writes_both_keys():
    """cache_balance sets gold + token keys with TTL."""
    r = RedisClient(url="redis://localhost:6379/0")
    r._redis = MagicMock()
    r._redis.set = AsyncMock()
    await r.cache_balance("alice", gold=100, token=50)
    assert r._redis.set.await_count == 2


@pytest.mark.asyncio
async def test_redis_client_get_cached_returns_none_when_no_keys():
    """get_cached_balance → None when both keys are absent."""
    r = RedisClient()
    r._redis = MagicMock()
    r._redis.get = AsyncMock(return_value=None)
    result = await r.get_cached_balance("alice")
    assert result is None