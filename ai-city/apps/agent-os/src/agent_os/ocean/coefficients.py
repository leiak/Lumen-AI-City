"""OCEAN → emotion baseline coefficient matrix.

Model: linear additive.
    baseline[emotion] = BASE[emotion] + Σ DIM_coeff[emotion][dim] × OCEAN[dim]
Negative values are clipped to 0 and the result normalized to sum 1.0 (see bias.py).

Rationale (2.0 stage 2 spec §10):

- High openness → curious (探索欲/新奇偏好), surprised (对新刺激反应强),
  thinking (抽象思维活跃); less neutral (抗拒无聊).
- High conscientiousness → thinking (系统性反思), neutral (稳重不易冲动);
  less surprised (反应不外露).
- High extraversion → happy (情绪外放), surprised (高反应性),
  embarrassed (社交场合多), neutral 持平.
- High agreeableness → happy (人际友好), embarrassed (在意他人评价);
  less angry/sad (敌意低/共情强).
- High neuroticism → sad (低落倾向), angry (易激惹),
  surprised (情绪反应强); less happy/neutral (情绪不稳定).

OCEAN dim ordering (matches npc_registry.OceanPersonality):
    (openness, conscientiousness, extraversion, agreeableness, neuroticism)
"""
from __future__ import annotations

from agent_os.stream.emotion_validator import ALLOWED_EMOTIONS

# Bias BASE[emotion] — default weight when OCEAN dims = 0.5 (neutral personality)
OCEAN_BIAS_BASE: dict[str, float] = {
    "happy":       0.10,
    "sad":         0.10,
    "angry":       0.08,  # bumped from 0.05 — was clipped too low at neutral personality
    "surprised":   0.10,
    "thinking":    0.10,
    "embarrassed": 0.08,
    "curious":     0.10,
    "neutral":     0.20,  # higher base — neutral is default mode
}

# Bias COEFFICIENTS[emotion] — 5-tuple of (o, c, e, a, n) multipliers
OCEAN_BIAS_COEFFICIENTS: dict[str, tuple[float, float, float, float, float]] = {
    # (openness, conscientiousness, extraversion, agreeableness, neuroticism)
    "happy":       ( 0.00,  0.00,  0.20,  0.15, -0.10),
    "sad":         ( 0.00,  0.00,  0.00, -0.05,  0.15),
    "angry":       ( 0.00,  0.00,  0.00, -0.13,  0.15),
    "surprised":   ( 0.10,  0.00,  0.15,  0.00,  0.10),
    "thinking":    ( 0.10,  0.20,  0.10,  0.00,  0.00),  # conscientiousness 0.15→0.20
    "embarrassed": ( 0.00,  0.00,  0.07,  0.07, -0.03),
    "curious":     ( 0.30,  0.00,  0.05,  0.00,  0.00),  # openness 0.20→0.30
    "neutral":     (-0.05,  0.20,  0.00,  0.00, -0.20),
}

# Defensive sanity check at import
assert set(OCEAN_BIAS_BASE) == set(ALLOWED_EMOTIONS), "OCEAN_BIAS_BASE keys must cover all 8 emotions"
assert set(OCEAN_BIAS_COEFFICIENTS) == set(ALLOWED_EMOTIONS), "OCEAN_BIAS_COEFFICIENTS keys must cover all 8 emotions"
for emo, coeffs in OCEAN_BIAS_COEFFICIENTS.items():
    assert len(coeffs) == 5, f"{emo} must have 5 OCEAN dims"