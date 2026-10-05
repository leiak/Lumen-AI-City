"""8 类 emotion 验证器；异常降级 neutral。"""
from __future__ import annotations

ALLOWED_EMOTIONS: frozenset[str] = frozenset({
    "happy", "sad", "angry", "surprised",
    "thinking", "embarrassed", "curious", "neutral",
})


class EmotionValidator:
    def validate(self, raw: str | None) -> str:
        if not raw:
            return "neutral"
        if raw in ALLOWED_EMOTIONS:
            return raw
        return "neutral"