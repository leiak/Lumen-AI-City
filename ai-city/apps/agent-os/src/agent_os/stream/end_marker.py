"""EndMarker: <end> tag / 3s 超时 → npc_say_stream_done。"""
from __future__ import annotations

import time


class EndMarker:
    def __init__(self, timeout_seconds: float = 3.0) -> None:
        self._done = False
        self._complete = False
        self._last_feed_at = time.time()
        self._timeout = timeout_seconds

    @property
    def done(self) -> bool:
        return self._done

    @property
    def complete(self) -> bool:
        return self._complete

    def feed(self, chunk: str) -> bool:
        """送入 token 块；返回是否触发 done。"""
        self._last_feed_at = time.time()
        if "<end>" in chunk:
            self._done = True
            self._complete = True
            return True
        return False

    def check_timeout(self) -> bool:
        """检查是否超时未收到 token → 隐式 done（complete=False）。"""
        if self._done:
            return False
        if time.time() - self._last_feed_at > self._timeout:
            self._done = True
            self._complete = False
            return True
        return False