"""Redis 节拍包发布器 + 失败 buffer 重试。"""
from __future__ import annotations

import asyncio
import json
import time
from typing import Any

CHANNEL_SAY_STREAM = "aicity:npc:say_stream"


class Publisher:
    def __init__(self, redis: Any, max_buffer: int = 100, retry_intervals: tuple[int, ...] = (1, 3, 10)) -> None:
        self._redis = redis
        self._max_buffer = max_buffer
        self._retry_intervals = retry_intervals
        self._buffer: list[str] = []

    async def publish_beat(
        self, *, npc_id: str, session_id: str, sentence_idx: int,
        text: str, emotion: str, trace_id: str,
    ) -> None:
        payload = {
            "type": "npc_say_stream",
            "npc_id": npc_id,
            "session_id": session_id,
            "sentence_idx": sentence_idx,
            "text": text,
            "emotion": emotion,
            "ts_ms": int(time.time() * 1000),
            "trace_id": trace_id,
        }
        await self._publish_with_retry(json.dumps(payload, ensure_ascii=False))

    async def publish_done(
        self, *, npc_id: str, session_id: str, sentence_count: int,
        complete: bool, trace_id: str,
    ) -> None:
        payload = {
            "type": "npc_say_stream_done",
            "npc_id": npc_id,
            "session_id": session_id,
            "sentence_count": sentence_count,
            "complete": complete,
            "ts_ms": int(time.time() * 1000),
            "trace_id": trace_id,
        }
        await self._publish_with_retry(json.dumps(payload, ensure_ascii=False))

    async def _publish_with_retry(self, payload: str) -> None:
        last_exc = None
        for delay in (0, *self._retry_intervals):
            if delay:
                await asyncio.sleep(delay)
            try:
                await self._redis.publish(CHANNEL_SAY_STREAM, payload)
                self._flush_buffer()
                return
            except Exception as e:
                last_exc = e
        # 全部失败 → 入 buffer
        if len(self._buffer) < self._max_buffer:
            self._buffer.append(payload)
        raise last_exc

    def _flush_buffer(self) -> None:
        """pub 成功时尝试 flush buffer（异步 fire-and-forget）。"""
        if not self._buffer:
            return
        pending = self._buffer[:]
        self._buffer.clear()

        async def _flush():
            for p in pending:
                try:
                    await self._redis.publish(CHANNEL_SAY_STREAM, p)
                except Exception:
                    self._buffer.append(p)
                    if len(self._buffer) >= self._max_buffer:
                        break

        asyncio.create_task(_flush())