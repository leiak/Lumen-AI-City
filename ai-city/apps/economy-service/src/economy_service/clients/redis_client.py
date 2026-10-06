"""redis-py async client for balance cache (econ:{user_id}:gold/token)."""
from __future__ import annotations
import os
import logging

logger = logging.getLogger(__name__)

REDIS_URL = os.environ.get("REDIS_URL", "redis://redis:6379/0")
GOLD_KEY = "econ:{user_id}:gold"
TOKEN_KEY = "econ:{user_id}:token"
DEFAULT_TTL = 3600  # 1h 缓存


class RedisClient:
    """redis-py async wrapper. start() 在 lifespan 内调一次."""

    def __init__(self, url: str = REDIS_URL) -> None:
        self.url = url
        self._redis = None

    async def start(self) -> None:
        from redis.asyncio import from_url  # lazy import
        self._redis = from_url(self.url, decode_responses=True)
        try:
            await self._redis.ping()
            logger.info("redis_client.connected: url=%s", self.url)
        except Exception as e:
            logger.warning("redis_client.ping failed: %s", e)
            self._redis = None

    async def stop(self) -> None:
        if self._redis is not None:
            try:
                await self._redis.aclose()
            except Exception as e:
                logger.warning("redis_client.stop failed: %s", e)
            self._redis = None

    async def cache_balance(self, user_id: str, gold: int, token: int) -> None:
        """写时双写缓存."""
        if self._redis is None:
            return
        try:
            await self._redis.set(GOLD_KEY.format(user_id=user_id), gold, ex=DEFAULT_TTL)
            await self._redis.set(TOKEN_KEY.format(user_id=user_id), token, ex=DEFAULT_TTL)
        except Exception as e:
            logger.warning("redis.cache_balance failed: %s", e)

    async def get_cached_balance(self, user_id: str) -> dict[str, int] | None:
        """读缓存; 缓存未命中返回 None (调用方 fallback PG)."""
        if self._redis is None:
            return None
        try:
            gold = await self._redis.get(GOLD_KEY.format(user_id=user_id))
            token = await self._redis.get(TOKEN_KEY.format(user_id=user_id))
            if gold is None and token is None:
                return None
            return {
                "gold_balance": int(gold) if gold is not None else 0,
                "token_balance": int(token) if token is not None else 0,
            }
        except Exception as e:
            logger.warning("redis.get_cached_balance failed: %s", e)
            return None