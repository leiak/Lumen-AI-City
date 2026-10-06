"""OCEAN → emotion baseline mapping unit tests (Phase A.1)."""
from __future__ import annotations

import pytest

from agent_os.npc_registry import OceanPersonality
from agent_os.ocean.bias import ocean_to_emotion_baseline
from agent_os.ocean.coefficients import OCEAN_BIAS_BASE, OCEAN_BIAS_COEFFICIENTS
from agent_os.stream.emotion_validator import ALLOWED_EMOTIONS


def _ocean(**kwargs) -> OceanPersonality:
    """Helper: OceanPersonality with default neutral 0.5 dims, override via kwargs."""
    defaults = {
        "openness": 0.5,
        "conscientiousness": 0.5,
        "extraversion": 0.5,
        "agreeableness": 0.5,
        "neuroticism": 0.5,
    }
    defaults.update(kwargs)
    return OceanPersonality(**defaults)


# 1. Output structure
def test_returns_all_8_emotions():
    result = ocean_to_emotion_baseline(_ocean())
    assert set(result.weights.keys()) == set(ALLOWED_EMOTIONS)
    assert len(result.weights) == 8


# 2. Sum-to-one
def test_weights_sum_to_one():
    result = ocean_to_emotion_baseline(_ocean())
    assert sum(result.weights.values()) == pytest.approx(1.0, abs=0.001)


# 3. Non-negative
def test_weights_non_negative():
    result = ocean_to_emotion_baseline(_ocean())
    for emo, w in result.weights.items():
        assert w >= 0.0, f"{emo} weight {w} is negative"


# 4. Rounded to 3 decimal places
def test_round_3dp():
    result = ocean_to_emotion_baseline(_ocean())
    for emo, w in result.weights.items():
        assert w == round(w, 3), f"{emo} weight {w} not rounded to 3 dp"


# 5. total_rows == 0 (baseline != history)
def test_total_rows_zero():
    result = ocean_to_emotion_baseline(_ocean())
    assert result.total_rows == 0


# 6. raw_counts empty (baseline != history)
def test_raw_counts_empty():
    result = ocean_to_emotion_baseline(_ocean())
    assert result.raw_counts == {}


# 7. High openness boosts curious
def test_high_openness_boosts_curious():
    high = ocean_to_emotion_baseline(_ocean(openness=0.9))
    low = ocean_to_emotion_baseline(_ocean(openness=0.1))
    assert high.weights["curious"] - low.weights["curious"] >= 0.10


# 8. High neuroticism boosts sad+angry
def test_high_neuroticism_boosts_sad_angry():
    high = ocean_to_emotion_baseline(_ocean(neuroticism=0.9))
    low = ocean_to_emotion_baseline(_ocean(neuroticism=0.1))
    delta = (high.weights["sad"] + high.weights["angry"]) - (
        low.weights["sad"] + low.weights["angry"]
    )
    assert delta >= 0.05


# 9. High extraversion boosts happy
def test_high_extraversion_boosts_happy():
    high = ocean_to_emotion_baseline(_ocean(extraversion=0.9))
    low = ocean_to_emotion_baseline(_ocean(extraversion=0.1))
    assert high.weights["happy"] - low.weights["happy"] >= 0.05


# 10. High conscientiousness boosts thinking
def test_high_conscientiousness_boosts_thinking():
    high = ocean_to_emotion_baseline(_ocean(conscientiousness=0.9))
    low = ocean_to_emotion_baseline(_ocean(conscientiousness=0.1))
    assert high.weights["thinking"] - low.weights["thinking"] >= 0.05


# 11. High agreeableness suppresses angry
def test_high_agreeableness_suppresses_angry():
    high = ocean_to_emotion_baseline(_ocean(agreeableness=0.9))
    low = ocean_to_emotion_baseline(_ocean(agreeableness=0.1))
    assert high.weights["angry"] < low.weights["angry"]
    delta = low.weights["angry"] - high.weights["angry"]
    assert delta >= 0.05


# 12. OCEAN dim out of [0, 1] raises
def test_occean_dim_out_of_range_raises():
    with pytest.raises(ValueError):
        ocean_to_emotion_baseline(_ocean(openness=1.1))
    with pytest.raises(ValueError):
        ocean_to_emotion_baseline(_ocean(neuroticism=-0.1))


# 13. Uniform personality gives reasonable baseline
def test_uniform_personality_gives_reasonable_baseline():
    result = ocean_to_emotion_baseline(_ocean())  # all 0.5
    for emo, w in result.weights.items():
        assert 0.05 <= w <= 0.40, f"{emo} weight {w} outside [0.05, 0.40]"


# 14. Coefficient keys match all 8 emotions
def test_coefficients_keys_match_emotions():
    assert set(OCEAN_BIAS_BASE) == set(ALLOWED_EMOTIONS)
    assert set(OCEAN_BIAS_COEFFICIENTS) == set(ALLOWED_EMOTIONS)
    assert len(OCEAN_BIAS_BASE) == 8
    assert len(OCEAN_BIAS_COEFFICIENTS) == 8