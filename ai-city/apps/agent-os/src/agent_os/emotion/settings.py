"""Emotion settings loaded from env vars. Single source of truth."""
from __future__ import annotations

import os
from dataclasses import dataclass


@dataclass(frozen=True)
class EmotionSettings:
    tau_player_seconds: float
    tau_global_seconds: float
    player_limit: int
    global_limit: int
    inject_enabled: bool

    @classmethod
    def from_env(cls) -> "EmotionSettings":
        tau_p = _parse_float("EMOTION_TAU_PLAYER_SECONDS", default=7200.0, min_val=1.0)
        tau_g = _parse_float("EMOTION_TAU_GLOBAL_SECONDS", default=86400.0, min_val=1.0)
        p_lim = _parse_int("EMOTION_PLAYER_LIMIT", default=50, min_val=1)
        g_lim = _parse_int("EMOTION_GLOBAL_LIMIT", default=100, min_val=1)
        enabled = os.getenv("EMOTION_INJECT_ENABLED", "true").lower() in ("true", "1", "yes")
        return cls(
            tau_player_seconds=tau_p,
            tau_global_seconds=tau_g,
            player_limit=p_lim,
            global_limit=g_lim,
            inject_enabled=enabled,
        )


def _parse_float(key: str, default: float, min_val: float) -> float:
    raw = os.getenv(key)
    if raw is None or not raw.strip():
        return default
    try:
        v = float(raw)
    except ValueError as e:
        raise ValueError(f"{key} must be float, got {raw!r}") from e
    if v < min_val:
        raise ValueError(f"{key} must be >= {min_val}, got {v}")
    return v


def _parse_int(key: str, default: int, min_val: int) -> int:
    raw = os.getenv(key)
    if raw is None or not raw.strip():
        return default
    try:
        v = int(raw)
    except ValueError as e:
        raise ValueError(f"{key} must be int, got {raw!r}") from e
    if v < min_val:
        raise ValueError(f"{key} must be >= {min_val}, got {v}")
    return v
