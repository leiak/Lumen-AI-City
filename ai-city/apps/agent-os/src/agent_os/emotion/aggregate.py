"""Emotion history aggregation with exponential time decay.

Pure math + formatting. No I/O. Tested in isolation (test_emotion_aggregate.py).
"""
from __future__ import annotations

import math
from collections import defaultdict
from dataclasses import dataclass
from datetime import datetime, timezone
from typing import Iterable


@dataclass(frozen=True)
class EmotionRow:
    """One row from memory_player_session with emotion tag."""
    emotion: str | None
    created_at: datetime  # must be tz-aware


@dataclass(frozen=True)
class EmotionDistribution:
    """Aggregated emotion distribution for prompt injection.

    weights: emotion → normalized weight (sums to 1.0; empty if no rows)
    raw_counts: emotion → unweighted row count (for debugging/logging)
    total_rows: total non-null emotion rows processed
    """
    weights: dict[str, float]
    raw_counts: dict[str, int]
    total_rows: int


def exp_decay_weight(age_seconds: float, tau_seconds: float) -> float:
    """Exponential decay: weight = exp(-Δt / τ).

    age = 0 → 1.0; age = τ → 0.368; age = 3τ → 0.050.

    Negative ages (future-dated rows from clock skew) are clamped to 0.
    """
    if tau_seconds <= 0:
        raise ValueError(f"tau_seconds must be > 0, got {tau_seconds}")
    return math.exp(-max(0.0, age_seconds) / tau_seconds)


def aggregate_distribution(
    rows: Iterable[EmotionRow],
    *,
    now: datetime | None = None,
    tau_seconds: float,
) -> EmotionDistribution:
    """Compute exp-decay-weighted emotion distribution from history rows."""
    if tau_seconds <= 0:
        raise ValueError(f"tau_seconds must be > 0, got {tau_seconds}")
    if now is None:
        now = datetime.now(timezone.utc)
    if now.tzinfo is None:
        raise ValueError("now must be tz-aware")

    weights: dict[str, float] = defaultdict(float)
    raw_counts: dict[str, int] = defaultdict(int)
    total = 0.0
    n = 0
    for row in rows:
        if not row.emotion:  # skip NULL / empty (pre-migration backfill)
            continue
        age = (now - row.created_at).total_seconds()
        w = exp_decay_weight(age, tau_seconds)
        weights[row.emotion] += w
        raw_counts[row.emotion] += 1
        total += w
        n += 1

    if total > 0:
        norm = {e: round(w / total, 3) for e, w in weights.items()}
    else:
        norm = {}

    return EmotionDistribution(
        weights=norm,
        raw_counts=dict(raw_counts),
        total_rows=n,
    )


def format_distribution_for_prompt(dist: EmotionDistribution) -> str:
    """Render distribution as compact text for prompt injection.

    Always shows all 8 emotion classes so the LLM sees the complete label set.
    Empty distribution → empty string (caller decides fallback text).
    """
    if dist.total_rows == 0:
        return ""
    from agent_os.stream.emotion_validator import ALLOWED_EMOTIONS
    parts = []
    for emo in sorted(ALLOWED_EMOTIONS):
        w = dist.weights.get(emo, 0.0)
        parts.append(f"{emo}={w:.3f}")
    return ", ".join(parts)


def top_emotion(dist: EmotionDistribution) -> str:
    """Highest-weighted emotion, or '无' if empty."""
    if not dist.weights:
        return "无"
    return max(dist.weights, key=dist.weights.get)