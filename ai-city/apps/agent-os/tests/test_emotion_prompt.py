"""Prompt injection tests for emotion persistence (B2-T07)."""
from __future__ import annotations

from agent_os.emotion.aggregate import EmotionDistribution
from agent_os.llm.prompts import get_npc_stream_prompt


def _make_dist(weights: dict[str, float], n: int = 1) -> EmotionDistribution:
    return EmotionDistribution(
        weights={k: round(v, 3) for k, v in weights.items()},
        raw_counts={k: n for k in weights},
        total_rows=n * len(weights),
    )


def test_prompt_includes_recent_distribution():
    recent = _make_dist({"happy": 0.6, "neutral": 0.4})
    prompt = get_npc_stream_prompt(
        "npc_wang_boss_001", "你好", [],
        recent_distribution=recent,
    )
    assert "happy=0.600" in prompt
    assert "neutral=0.400" in prompt
    assert "【最近情绪氛围" in prompt


def test_prompt_includes_global_distribution():
    global_d = _make_dist({"sad": 0.7, "neutral": 0.3})
    prompt = get_npc_stream_prompt(
        "npc_wang_boss_001", "你好", [],
        global_distribution=global_d,
    )
    assert "sad=0.700" in prompt


def test_prompt_first_interaction_shows_empty_marker():
    empty = EmotionDistribution(weights={}, raw_counts={}, total_rows=0)
    prompt = get_npc_stream_prompt(
        "npc_wang_boss_001", "你好", [],
        recent_distribution=empty,
        global_distribution=empty,
    )
    assert "首次对话" in prompt or "无历史情绪" in prompt


def test_prompt_no_injection_when_distributions_none():
    """Backward compat: omitting distributions yields pre-B2 prompt shape."""
    prompt = get_npc_stream_prompt("npc_wang_boss_001", "你好", [])
    assert "【最近情绪氛围" not in prompt  # no section injected
    assert "happy=0.600" not in prompt
