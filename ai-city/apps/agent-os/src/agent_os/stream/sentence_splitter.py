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
        """送入 token 块列表，yield 切好的 (text, emotion) 元组。"""
        for chunk in chunks:
            self._buffer += chunk

            # 检测 <end> 标记
            if _END_MARKER in self._buffer:
                self._done = True
                # end 之前的所有句子先 yield
                self._buffer = self._buffer.replace(_END_MARKER, "")

            # 提取所有闭合 tag 对
            for match in _SENTENCE_PATTERN.finditer(self._buffer):
                emotion = match.group(1)
                text = match.group(2).strip()
                if text:
                    yield (text, emotion)

            # 清理已处理的 buffer（保留未闭合部分）
            last_end = 0
            for match in _SENTENCE_PATTERN.finditer(self._buffer):
                last_end = match.end()
            self._buffer = self._buffer[last_end:]

    def flush(self) -> Iterator[tuple[str, str]]:
        """EOF 时强制 flush buffer 中残留（无 emotion tag 的孤儿句）。"""
        if self._buffer.strip():
            # 残留当 neutral
            yield (self._buffer.strip(), "neutral")
            self._buffer = ""
