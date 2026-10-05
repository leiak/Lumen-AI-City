"""把 LLM 流式 token 切成 (text, emotion) 元组流。

XML tag 格式：<emotion=happy>来了您嘞！</emotion>
EOF 标记：<end>

缓冲 token 直到闭合 tag，提取 (text, emotion) 元组。
"""
from __future__ import annotations

import re
from typing import Iterator

_SENTENCE_PATTERN = re.compile(
    r'<emotion=([a-z]+)>(.*?)</emotion>', re.DOTALL
)
_END_MARKER = "<end>"


class SentenceSplitter:
    """流式 token → (text, emotion) 元组生成器。"""

    def __init__(self) -> None:
        self._buffer = ""
        self._done = False

    @property
    def done(self) -> bool:
        return self._done

    def feed(self, chunks: list[str]) -> Iterator[tuple[str, str]]:
        """送入 token 块列表，yield 切好的 (text, emotion) 元组。

        State-mutation safety: matches are snapshotted, buffer is cleared,
        then we yield from the snapshot. A consumer breaking mid-iteration
        will NOT cause re-emission on the next call.
        """
        for chunk in chunks:
            if self._done:
                return  # hard stop after <end>; spec §2.4 EOF semantics
            self._buffer += chunk

            # 检测 <end> 标记
            if _END_MARKER in self._buffer:
                self._done = True
                self._buffer = self._buffer.replace(_END_MARKER, "")

            # 单 pass：收集所有完整 tag 到 snapshot，再 mutate buffer
            snapshot = list(_SENTENCE_PATTERN.finditer(self._buffer))
            if not snapshot:
                continue

            # 截断 buffer 到最后一个 match.end()
            self._buffer = self._buffer[snapshot[-1].end():]

            # 从 snapshot yield（buffer 已被 mutate，consumer break 安全）
            for match in snapshot:
                emotion = match.group(1)
                text = match.group(2).strip()
                if text:
                    yield (text, emotion)

    def flush(self) -> Iterator[tuple[str, str]]:
        """EOF 时强制 flush buffer 中残留（无 emotion tag 的孤儿句）。

        部分闭合的 tag（如 <emotion=happy>）被剥离，避免泄漏 raw tag。
        """
        residual = self._buffer
        if not residual.strip():
            return
        # 剥离未闭合的 opening emotion tag（包括末尾的 partial 和开头的 orphan）
        residual = re.sub(r'<emotion=[^>]+>', '', residual)
        # 剥离末尾残缺未闭合 tag（如 <emotion=happy）
        residual = re.sub(r'<emotion=[^>]*$', '', residual)
        residual = residual.strip()
        if residual:
            yield (residual, "neutral")
        self._buffer = ""
