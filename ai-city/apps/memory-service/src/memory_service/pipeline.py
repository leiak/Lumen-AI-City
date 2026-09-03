"""三层记忆管道。

感官缓存 (Redis 5min) → 情节记忆 (PG 30d) → 语义记忆 (Milvus 永久)。
"""
from __future__ import annotations

import json
import logging
from datetime import datetime, timedelta, timezone

import redis.asyncio as redis

logger = logging.getLogger(__name__)


class MemoryPipeline:
    def __init__(self, redis_url: str, pg_url: str, milvus_url: str) -> None:
        self.redis = redis.from_url(redis_url, decode_responses=True)
        # TODO: pg + milvus clients
        self.pg_url = pg_url
        self.milvus_url = milvus_url

    async def remember(self, agent_id: str, percept: dict, importance: int = 3) -> None:
        """写入三层记忆。"""
        now = datetime.now(timezone.utc)

        # 1. 感官缓存（5min TTL，最多 10 条）
        key = f"sens:{agent_id}"
        await self.redis.lpush(key, json.dumps(percept, ensure_ascii=False))
        await self.redis.ltrim(key, 0, 9)
        await self.redis.expire(key, 300)

        # 2. 情节记忆（30d TTL，importance >= 4 才持久化）
        if importance >= 4:
            # TODO: 写入 PG episodic_memory 表
            pass

        # 3. 语义记忆（永久，importance >= 5 才向量化）
        if importance >= 5:
            # TODO: 写入 Milvus semantic_memory collection
            pass

    async def recall(self, agent_id: str, query: str, top_k: int = 5) -> list[dict]:
        """检索记忆：先感官 → 情节 → 语义。"""
        results: list[dict] = []

        # 1. 感官
        sens = await self.redis.lrange(f"sens:{agent_id}", 0, 9)
        for s in sens:
            results.append({"source": "sensory", "content": json.loads(s), "score": 1.0})

        # 2. 情节
        # TODO: PG 全文检索 + 时间衰减

        # 3. 语义
        # TODO: Milvus 向量检索

        return results[:top_k]
