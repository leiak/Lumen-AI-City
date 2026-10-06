"""60min TTL 内存 session map + 重连补帧 buffer。

Stage 2 (C2 T31 决议) 起 wire 到 ``dispatcher.say_stream()`` — 每个 beat 写 buffer，
done 时 mark_done。Stage 3 暴露 ``GET /v1/npc/sessions/{sid}/buffer?from_idx=N``
供玩家 WS 断线重连时拉帧。

测试 ``tests/stream/test_session_store.py`` 保留 — 验证 TTL 过期 / buffer 过滤 /
session 隔离等不变量。
"""
from __future__ import annotations

import time
import uuid
from dataclasses import dataclass, field

from agent_os.errors import R015SessionNotFound


@dataclass
class _Session:
    npc_id: str
    created_at: float
    buffer: list[dict] = field(default_factory=list)
    done: bool = False       # mark_done 是否被调用过
    complete: bool = False   # mark_done(complete=...) 的值


class SessionStore:
    def __init__(self, ttl_seconds: int = 60 * 60) -> None:
        self._sessions: dict[str, _Session] = {}
        self._ttl = ttl_seconds

    def create(self, npc_id: str) -> str:
        sid = f"sess-{uuid.uuid4().hex[:12]}"
        self._sessions[sid] = _Session(
            npc_id=npc_id, created_at=time.time()
        )
        return sid

    def append(self, sid: str, sentence_idx: int, text: str, emotion: str) -> None:
        self._require(sid)
        self._sessions[sid].buffer.append({
            "sentence_idx": sentence_idx,
            "text": text,
            "emotion": emotion,
        })

    def get_buffer(self, sid: str, from_idx: int = 0) -> list[dict]:
        self._require(sid)
        return [b for b in self._sessions[sid].buffer if b["sentence_idx"] >= from_idx]

    def mark_done(self, sid: str, complete: bool = True) -> None:
        self._require(sid)
        sess = self._sessions[sid]
        sess.done = True
        sess.complete = complete

    def is_done(self, sid: str) -> bool:
        """``mark_done`` 是否被调用过；session 不存在或过期返 False。"""
        try:
            self._require(sid)
        except R015SessionNotFound:
            return False
        return self._sessions[sid].done

    def is_complete(self, sid: str) -> bool:
        """``mark_done(complete=True)`` 是否被调用过（流正常结束）。
        False = 未结束 / 异常结束 / session 不存在。
        """
        try:
            self._require(sid)
        except R015SessionNotFound:
            return False
        return self._sessions[sid].complete

    def _require(self, sid: str) -> None:
        sess = self._sessions.get(sid)
        if sess is None:
            raise R015SessionNotFound
        if time.time() - sess.created_at > self._ttl:
            del self._sessions[sid]
            raise R015SessionNotFound
