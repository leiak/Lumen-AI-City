# apps/economy-service/src/economy_service/services/idempotency.py
"""Redis-backed idempotency key store. First-response cache, TTL 24h."""
from __future__ import annotations

import json
from typing import Any


class IdempotencyStore:
    def __init__(self, redis_client) -> None:
        self.redis = redis_client
        self.ttl = 86400  # 24h

    def _key(self, scope: str, key: str) -> str:
        return f"econ:idem:{scope}:{key}"

    async def check_and_set(self, scope: str, key: str, value: Any) -> bool:
        """Returns True if newly set (proceed), False if duplicate (return cached)."""
        return await self.redis.set(
            self._key(scope, key),
            json.dumps(value),
            nx=True,
            ex=self.ttl,
        )

    async def get_cached(self, scope: str, key: str) -> Any | None:
        raw = await self.redis.get(self._key(scope, key))
        return json.loads(raw) if raw else None
