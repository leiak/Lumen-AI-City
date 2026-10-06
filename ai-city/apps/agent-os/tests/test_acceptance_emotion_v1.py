"""Acceptance binary unit tests for A.5 (OCEAN baseline E2E).

Mocked verification of:
  Step 1: enabled NPC templates have baseline_emotion_distribution cached
  Step 2: reference NPC's happy weight is in a reasonable range
  Step 3: extraversion 0.9 vs 0.1 → happy delta ≥ 0.05
  Step 4: openness 0.9 vs 0.1 → curious delta ≥ 0.10
  Step 5: prompt with baseline_distribution contains 【人格基线情绪】 section
  Step 6: prompt without baseline_distribution does NOT contain baseline section

Run:
  cd apps/agent-os
  PYTHONPATH=src .venv/Scripts/pytest.exe tests/test_acceptance_emotion_v1.py -v
"""
from __future__ import annotations

import sys
from pathlib import Path

import pytest

# Path setup for direct imports (matches test_acceptance_emotion.py pattern)
SRC = Path(__file__).parent.parent / "src"
if str(SRC) not in sys.path:
    sys.path.insert(0, str(SRC))

from agent_os.npc_registry import OceanPersonality
from agent_os.emotion.aggregate import EmotionDistribution
from agent_os.ocean.bias import ocean_to_emotion_baseline


def test_step1_templates_have_baseline():
    """Step 1: enabled NPC templates should have baseline_emotion_distribution cached."""
    from agent_os.npc_registry import NpcRegistry
    templates_dir = Path(__file__).parent.parent.parent.parent / "packages" / "npc-templates"
    reg = NpcRegistry(templates_dir)
    enabled = reg.list_enabled()
    assert len(enabled) > 0, "no enabled NPC templates"
    with_baseline = [t for t in enabled if t.baseline_emotion_distribution is not None]
    # At least one NPC with personality set (wang_boss/grace_healer have personality)
    assert len(with_baseline) > 0, "no NPC has baseline computed"


def test_step2_baseline_happy_default():
    """Step 2: reference NPC's happy weight is reasonable (0.10-0.30)."""
    ocean = OceanPersonality(
        openness=0.3, conscientiousness=0.85,
        extraversion=0.7, agreeableness=0.4, neuroticism=0.3,
    )
    baseline = ocean_to_emotion_baseline(ocean)
    happy = baseline.weights["happy"]
    assert 0.10 <= happy <= 0.30, f"happy={happy} outside [0.10, 0.30]"


def test_step3_extraversion_shifts_happy():
    """Step 3: extraversion 0.9 vs 0.1, happy delta ≥ 0.05."""
    high = ocean_to_emotion_baseline(OceanPersonality(
        openness=0.5, conscientiousness=0.5,
        extraversion=0.9, agreeableness=0.5, neuroticism=0.5,
    ))
    low = ocean_to_emotion_baseline(OceanPersonality(
        openness=0.5, conscientiousness=0.5,
        extraversion=0.1, agreeableness=0.5, neuroticism=0.5,
    ))
    delta = high.weights["happy"] - low.weights["happy"]
    assert delta >= 0.05, f"happy delta {delta} < 0.05"


def test_step4_openness_shifts_curious():
    """Step 4: openness 0.9 vs 0.1, curious delta ≥ 0.10."""
    high = ocean_to_emotion_baseline(OceanPersonality(
        openness=0.9, conscientiousness=0.5,
        extraversion=0.5, agreeableness=0.5, neuroticism=0.5,
    ))
    low = ocean_to_emotion_baseline(OceanPersonality(
        openness=0.1, conscientiousness=0.5,
        extraversion=0.5, agreeableness=0.5, neuroticism=0.5,
    ))
    delta = high.weights["curious"] - low.weights["curious"]
    assert delta >= 0.10, f"curious delta {delta} < 0.10"


def test_step5_prompt_includes_baseline_section():
    """Step 5: prompt with baseline_distribution contains 【人格基线情绪】 section.

    total_rows=1 (non-zero) is required so ``_render_baseline_section`` emits the
    block (it short-circuits on total_rows=0; see prompts.py docstring).
    """
    from agent_os.llm.prompts import get_npc_stream_prompt

    baseline = EmotionDistribution(
        weights={"happy": 0.20, "sad": 0.05, "angry": 0.05, "surprised": 0.10,
                 "thinking": 0.15, "embarrassed": 0.05, "curious": 0.15, "neutral": 0.25},
        raw_counts={"happy": 1, "sad": 1, "angry": 1, "surprised": 1,
                    "thinking": 1, "embarrassed": 1, "curious": 1, "neutral": 1},
        total_rows=8,
    )
    prompt = get_npc_stream_prompt(
        npc_id="npc_a_wang_boss_001",
        player_input="客官您来了",
        npc_context=[],
        baseline_distribution=baseline,
    )
    assert "【人格基线情绪" in prompt
    assert "OCEAN" in prompt or "偏好" in prompt


def test_step6_kill_switch_disables_baseline():
    """Step 6: prompt without baseline_distribution does NOT contain baseline section."""
    from agent_os.llm.prompts import get_npc_stream_prompt

    prompt = get_npc_stream_prompt(
        npc_id="npc_a_wang_boss_001",
        player_input="客官您来了",
        npc_context=[],
    )
    assert "【人格基线情绪" not in prompt, "kill switch should disable baseline injection"