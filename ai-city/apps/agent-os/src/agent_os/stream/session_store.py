"""60min TTL 内存 session map + 重连补帧 buffer。"""
from __future__ import annotations

import time
import uuid
from dataclasses import dataclass, field

from agent_os.errors import R_015


@dataclass
class _Session:
    npc_id: str
    created_at: float
    buffer: list[dict] = field(default_factory=list)


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
        # done 是元数据；存储为 _Session 字段扩展（plan 阶段不实现具体细节）

    def _require(self, sid: str) -> None:
        sess = self._sessions.get(sid)
        if sess is None:
            raise R_015
        if time.time() - sess.created_at > self._ttl:
            del self._sessions[sid]
            raise R_015
