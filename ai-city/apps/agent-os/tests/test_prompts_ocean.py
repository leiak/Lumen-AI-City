"""Prompt injection tests for OCEAN personality baseline (Phase A.4).

Verifies:
- baseline_distribution=None → no 【人格基线情绪】 section injected
- Empty baseline (total_rows=0) → no section injected
- Populated baseline → section with all 8 emotions + format_distribution_for_prompt
- Section placement: after 【最近情绪氛围】, before history turns
- Backward-compat: all None → byte-identical pre-A.4 prompt shape
"""
from __future__ import annotations

from agent_os.emotion.aggregate import EmotionDistribution
from agent_os.llm.prompts import get_npc_stream_prompt
from agent_os.stream.emotion_validator import ALLOWED_EMOTIONS


def _make_dist(
    weights: dict[str, float], n: int = 1,
) -> EmotionDistribution:
    return EmotionDistribution(
        weights={k: round(v, 3) for k, v in weights.items()},
        raw_counts={k: n for k in weights},
        total_rows=n * len(weights),
    )


def test_no_baseline_returns_no_section():
    """baseline_distribution=None → no 【人格基线情绪】 section in prompt."""
    prompt = get_npc_stream_prompt(
        "npc_wang_boss_001", "你好", [],
        baseline_distribution=None,
    )
    assert "【人格基线情绪" not in prompt


def test_baseline_empty_distribution_returns_no_section():
    """baseline_distribution with total_rows=0 → no section rendered."""
    empty = EmotionDistribution(weights={}, raw_counts={}, total_rows=0)
    prompt = get_npc_stream_prompt(
        "npc_wang_boss_001", "你好", [],
        baseline_distribution=empty,
    )
    assert "【人格基线情绪" not in prompt


def test_baseline_with_weights_renders_section():
    """Populated baseline → 【人格基线情绪】 section is present."""
    baseline = _make_dist({"happy": 0.5, "neutral": 0.5})
    prompt = get_npc_stream_prompt(
        "npc_wang_boss_001", "你好", [],
        baseline_distribution=baseline,
    )
    assert "【人格基线情绪" in prompt
    # Should also contain the rendered weight values
    assert "happy=0.500" in prompt
    assert "neutral=0.500" in prompt


def test_baseline_section_shows_all_8_emotions():
    """Section renders all 8 allowed emotion labels (via format_distribution_for_prompt)."""
    baseline = _make_dist({"happy": 0.5, "neutral": 0.5})
    prompt = get_npc_stream_prompt(
        "npc_wang_boss_001", "你好", [],
        baseline_distribution=baseline,
    )
    # Every allowed emotion label must appear at least once in the baseline section
    for emo in ALLOWED_EMOTIONS:
        assert f"{emo}=" in prompt, f"missing emotion label: {emo}"


def test_baseline_section_after_emotion_history():
    """When both recent + baseline populated: 【最近情绪氛围】 BEFORE 【人格基线情绪】."""
    recent = _make_dist({"happy": 0.6, "neutral": 0.4})
    baseline = _make_dist({"sad": 0.5, "neutral": 0.5})
    prompt = get_npc_stream_prompt(
        "npc_wang_boss_001", "你好", [],
        recent_distribution=recent,
        baseline_distribution=baseline,
    )
    history_idx = prompt.find("【最近情绪氛围")
    baseline_idx = prompt.find("【人格基线情绪")
    assert history_idx != -1
    assert baseline_idx != -1
    assert history_idx < baseline_idx, (
        f"emotion history must precede baseline: "
        f"history={history_idx} baseline={baseline_idx}"
    )


def test_no_kwargs_byte_identical_to_pre_a4():
    """All optional kwargs None → prompt byte-identical to call without new kwarg.

    A.4 only adds a new optional kwarg; omitting it must yield the same string
    as calling with baseline_distribution=None (the new default).
    """
    history = [{"role": "user", "content": "hi"}]
    prompt_no_new_kwarg = get_npc_stream_prompt(
        "npc_wang_boss_001", "你好", history,
    )
    prompt_with_new_kwarg = get_npc_stream_prompt(
        "npc_wang_boss_001", "你好", history,
        baseline_distribution=None,
    )
    assert prompt_no_new_kwarg == prompt_with_new_kwarg
    # And the shape itself must match pre-A.4: no double newline after </system>
    assert prompt_no_new_kwarg.startswith("<system>")
    assert "</system>\n\n" not in prompt_no_new_kwarg


def test_baseline_section_mentions_occean():
    """Section text mentions OCEAN (or 偏好) so LLM sees the section's purpose."""
    baseline = _make_dist({"happy": 0.5, "neutral": 0.5})
    prompt = get_npc_stream_prompt(
        "npc_wang_boss_001", "你好", [],
        baseline_distribution=baseline,
    )
    # Either literal "OCEAN" or Chinese "偏好" should appear in the baseline section
    has_marker = "OCEAN" in prompt or "偏好" in prompt
    assert has_marker, "baseline section should mention OCEAN or 偏好"


def test_baseline_section_placement_before_history():
    """【人格基线情绪】 appears BEFORE <user>...</user> and BEFORE history turns."""
    history = [
        {"role": "user", "content": "第一条历史"},
        {"role": "assistant", "content": "第一条回复"},
    ]
    baseline = _make_dist({"happy": 0.5, "neutral": 0.5})
    prompt = get_npc_stream_prompt(
        "npc_wang_boss_001", "新的输入", history,
        baseline_distribution=baseline,
    )
    baseline_idx = prompt.find("【人格基线情绪")
    user_idx = prompt.find("<user>新的输入</user>")
    history_user_idx = prompt.find("<user>第一条历史</user>")
    assert baseline_idx != -1
    assert user_idx != -1
    assert history_user_idx != -1
    assert baseline_idx < user_idx, "baseline must come before new <user> input"
    assert baseline_idx < history_user_idx, "baseline must come before history turns"