"""Dispatcher OCEAN baseline → prompt injection tests (Phase A.4).

Verifies:
- OCEAN_BIAS_ENABLED=False → no baseline injected (byte-identical pre-A.4)
- OCEAN_BIAS_ENABLED=True + registry with baseline → baseline section in prompt
- Unknown npc_id raises KeyError → dispatcher logs warning + continues no-baseline
- No registry configured → no baseline injected (graceful no-op)
"""
from __future__ import annotations

import asyncio
from unittest.mock import AsyncMock, MagicMock

import pytest

from agent_os.dispatcher import ActionDispatcher
from agent_os.emotion.aggregate import EmotionDistribution
from agent_os.emotion.settings import EmotionSettings
from agent_os.npc_registry import NpcTemplate


def _make_settings(ocean_bias_enabled: bool) -> EmotionSettings:
    return EmotionSettings(
        tau_player_seconds=7200.0,
        tau_global_seconds=86400.0,
        player_limit=50,
        global_limit=100,
        inject_enabled=False,  # emotion history kill switch is OFF in these tests
        ocean_bias_enabled=ocean_bias_enabled,
    )


def _make_baseline_dist() -> EmotionDistribution:
    return EmotionDistribution(
        weights={
            "happy": 0.3, "sad": 0.1, "angry": 0.05, "surprised": 0.1,
            "thinking": 0.2, "embarrassed": 0.05, "curious": 0.1, "neutral": 0.1,
        },
        raw_counts={k: 1 for k in [
            "happy", "sad", "angry", "surprised",
            "thinking", "embarrassed", "curious", "neutral",
        ]},
        total_rows=8,
    )


def _make_template(npc_id: str, baseline: EmotionDistribution | None) -> NpcTemplate:
    return NpcTemplate(
        npc_id=npc_id,
        name="Wang Boss",
        enabled=True,
        baseline_emotion_distribution=baseline,
    )


def _capture_prompt(
    npc_id: str = "npc_wang_boss_001",
    *,
    ocean_bias_enabled: bool,
    npc_registry,
    emotion_repo=None,
) -> str:
    """Drive say_stream with a fake LLM; return the prompt it received."""
    captured: dict[str, str] = {}

    async def fake_stream(req):
        captured["prompt"] = req.prompt
        yield {"text": "<emotion=happy>好</emotion>", "finish_reason": None}
        yield {"text": "<end>", "finish_reason": "stop"}

    llm = MagicMock()
    llm.stream = fake_stream
    publisher = MagicMock()
    publisher.publish_beat = AsyncMock()
    publisher.publish_done = AsyncMock()

    dispatcher = ActionDispatcher(
        llm_client=llm,
        publisher=publisher,
        emotion_repo=emotion_repo,
        emotion_settings=_make_settings(ocean_bias_enabled),
        npc_registry=npc_registry,
    )

    async def run() -> None:
        async for _ in dispatcher.say_stream(
            npc_id=npc_id,
            player_input="你好",
            npc_context=[],
            session_id="sess-ocean",
            trace_id="tr-ocean",
        ):
            pass

    asyncio.run(run())
    return captured["prompt"]


# ---------------------------------------------------------------------------
# Test 1: OCEAN_BIAS_ENABLED=False → no baseline section
# ---------------------------------------------------------------------------


def test_ocean_bias_disabled_no_baseline_in_prompt():
    """OCEAN_BIAS_ENABLED=False + populated registry → no baseline in prompt."""
    baseline = _make_baseline_dist()
    registry = MagicMock()
    registry.get = MagicMock(return_value=_make_template("npc_wang_boss_001", baseline))

    prompt = _capture_prompt(
        ocean_bias_enabled=False, npc_registry=registry,
    )
    assert "【人格基线情绪" not in prompt
    # registry.get should not have been called when kill switch is off
    registry.get.assert_not_called()


# ---------------------------------------------------------------------------
# Test 2: OCEAN_BIAS_ENABLED=True + populated registry → baseline in prompt
# ---------------------------------------------------------------------------


def test_ocean_bias_enabled_baseline_in_prompt():
    """OCEAN_BIAS_ENABLED=True + populated registry → baseline section in prompt."""
    baseline = _make_baseline_dist()
    registry = MagicMock()
    registry.get = MagicMock(return_value=_make_template("npc_wang_boss_001", baseline))

    prompt = _capture_prompt(
        ocean_bias_enabled=True, npc_registry=registry,
    )
    assert "【人格基线情绪" in prompt
    # Registry was consulted for this baseline
    registry.get.assert_called_once_with("npc_wang_boss_001")


# ---------------------------------------------------------------------------
# Test 3: OCEAN_BIAS_ENABLED=True + unknown npc_id → no crash, no baseline
# ---------------------------------------------------------------------------


def test_ocean_bias_enabled_unknown_npc_id_no_crash(caplog):
    """OCEAN_BIAS_ENABLED=True + registry raises KeyError → warn + no baseline."""
    import logging

    registry = MagicMock()
    registry.get = MagicMock(side_effect=KeyError("npc_unknown"))

    with caplog.at_level(logging.WARNING):
        prompt = _capture_prompt(
            npc_id="npc_unknown",
            ocean_bias_enabled=True,
            npc_registry=registry,
        )

    # Dispatcher logs a warning, does not raise, and renders no baseline section
    assert "【人格基线情绪" not in prompt
    # Some warning mentioning OCEAN baseline should be present
    assert any(
        "OCEAN" in rec.message and "baseline" in rec.message.lower()
        for rec in caplog.records
    )


# ---------------------------------------------------------------------------
# Test 4: No registry configured → no baseline injected
# ---------------------------------------------------------------------------


def test_no_registry_no_baseline_in_prompt():
    """OCEAN_BIAS_ENABLED=True but npc_registry=None → no baseline injected."""
    prompt = _capture_prompt(
        ocean_bias_enabled=True, npc_registry=None,
    )
    assert "【人格基线情绪" not in prompt