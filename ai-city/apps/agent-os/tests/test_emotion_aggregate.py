"""Aggregate module unit tests."""
from __future__ import annotations
from datetime import datetime, timedelta, timezone

import pytest

from agent_os.emotion.aggregate import (
    EmotionDistribution,
    EmotionRow,
    aggregate_distribution,
    exp_decay_weight,
    format_distribution_for_prompt,
    top_emotion,
)


NOW = datetime(2026, 10, 6, 12, 0, 0, tzinfo=timezone.utc)


def make_row(emo: str, age_seconds: float) -> EmotionRow:
    return EmotionRow(emotion=emo, created_at=NOW - timedelta(seconds=age_seconds))


def test_exp_decay_weight_zero_age():
    assert exp_decay_weight(0.0, 7200.0) == pytest.approx(1.0, abs=1e-9)


def test_exp_decay_weight_one_tau():
    assert exp_decay_weight(7200.0, 7200.0) == pytest.approx(0.367879, abs=1e-4)


def test_exp_decay_weight_three_tau():
    assert exp_decay_weight(21600.0, 7200.0) == pytest.approx(0.049787, abs=1e-4)


def test_exp_decay_weight_tau_zero_raises():
    with pytest.raises(ValueError):
        exp_decay_weight(100.0, 0.0)


def test_exp_decay_weight_negative_age_clamped():
    """Future-dated rows (clock skew) treated as age=0."""
    assert exp_decay_weight(-100.0, 7200.0) == pytest.approx(1.0, abs=1e-9)


def test_aggregate_empty_rows():
    dist = aggregate_distribution([], tau_seconds=7200.0)
    assert dist.weights == {}
    assert dist.raw_counts == {}
    assert dist.total_rows == 0


def test_aggregate_uniform_age_uniform_distribution():
    rows = [make_row("happy", 60.0) for _ in range(5)]
    dist = aggregate_distribution(rows, now=NOW, tau_seconds=7200.0)
    assert dist.total_rows == 5
    assert dist.weights == {"happy": 1.0}


def test_aggregate_skips_null_emotion():
    rows = [
        EmotionRow(emotion="", created_at=NOW),
        EmotionRow(emotion=None, created_at=NOW),  # type: ignore[arg-type]
        make_row("happy", 60.0),
    ]
    dist = aggregate_distribution(rows, now=NOW, tau_seconds=7200.0)
    assert dist.total_rows == 1
    assert dist.weights == {"happy": 1.0}


def test_aggregate_recent_dominates():
    """9 old sad + 1 new happy → happy > sad (exp decay makes old negligible)."""
    rows = [make_row("sad", 86400.0) for _ in range(9)]
    rows.append(make_row("happy", 10.0))
    dist = aggregate_distribution(rows, now=NOW, tau_seconds=3600.0)
    assert dist.weights["happy"] > dist.weights["sad"]


def test_format_distribution_all_8_classes():
    """Empty distribution renders all 8 emotion classes with 0.000."""
    dist = EmotionDistribution(weights={"happy": 1.0}, raw_counts={"happy": 1}, total_rows=1)
    text = format_distribution_for_prompt(dist)
    for emo in ("happy", "sad", "angry", "surprised",
                "thinking", "embarrassed", "curious", "neutral"):
        assert f"{emo}=" in text, f"missing {emo} in {text}"


def test_format_distribution_empty_renders_empty_marker():
    dist = EmotionDistribution(weights={}, raw_counts={}, total_rows=0)
    # Caller decides what to do with empty; function just returns empty string
    assert format_distribution_for_prompt(dist) == ""


def test_top_emotion_picks_highest_weight():
    dist = EmotionDistribution(
        weights={"happy": 0.6, "neutral": 0.3, "sad": 0.1},
        raw_counts={}, total_rows=10,
    )
    assert top_emotion(dist) == "happy"


def test_top_emotion_empty_returns_placeholder():
    dist = EmotionDistribution(weights={}, raw_counts={}, total_rows=0)
    assert top_emotion(dist) == "无"