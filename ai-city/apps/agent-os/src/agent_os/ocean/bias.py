"""OCEAN → emotion baseline mapping (linear-additive model)."""
from __future__ import annotations

import logging
from agent_os.emotion.aggregate import EmotionDistribution
from agent_os.npc_registry import OceanPersonality
from agent_os.ocean.coefficients import OCEAN_BIAS_BASE, OCEAN_BIAS_COEFFICIENTS
from agent_os.stream.emotion_validator import ALLOWED_EMOTIONS

_logger = logging.getLogger(__name__)

OCEAN_DIM_KEYS = ("openness", "conscientiousness", "extraversion", "agreeableness", "neuroticism")


def ocean_to_emotion_baseline(ocean: OceanPersonality) -> EmotionDistribution:
    """Compute 8-emotion baseline weights from an OCEAN personality vector.

    Linear additive model: baseline[emotion] = BASE + Σ DIM_coeff · OCEAN[dim].
    Negative raw values are clipped to 0.0; result normalized to sum 1.0.

    Args:
        ocean: OCEAN 5-dim personality vector (each dim ∈ [0, 1]).

    Returns:
        EmotionDistribution with `weights` summing to 1.0 (rounded to 3 dp),
        `raw_counts` empty, `total_rows=0` (baseline ≠ history rows).

    Raises:
        ValueError: if any OCEAN dim is outside [0, 1].
    """
    ocean_dims = (
        ocean.openness, ocean.conscientiousness, ocean.extraversion,
        ocean.agreeableness, ocean.neuroticism,
    )
    for dim_name, dim_value in zip(OCEAN_DIM_KEYS, ocean_dims):
        if not 0.0 <= dim_value <= 1.0:
            raise ValueError(
                f"OCEAN dim {dim_name}={dim_value} out of [0, 1] range"
            )

    raw: dict[str, float] = {}
    for emo in ALLOWED_EMOTIONS:
        base = OCEAN_BIAS_BASE[emo]
        coeffs = OCEAN_BIAS_COEFFICIENTS[emo]
        value = base + sum(c * d for c, d in zip(coeffs, ocean_dims))
        raw[emo] = max(0.0, value)  # clip negative

    total = sum(raw.values())
    if total <= 0:
        # Defensive: shouldn't happen with reasonable coefficients, but
        # avoid divide-by-zero. Fall back to uniform distribution.
        _logger.warning(
            "OCEAN bias produced all-zero weights (ocean=%s); falling back to uniform",
            ocean_dims,
        )
        n = len(ALLOWED_EMOTIONS)
        weights = {e: round(1.0 / n, 3) for e in ALLOWED_EMOTIONS}
    else:
        weights = {e: round(v / total, 3) for e, v in raw.items()}
        # Compensate rounding drift: 8 × round-to-3dp may sum to 0.999 instead
        # of 1.0. Redistribute residual to the largest weight so the prompt
        # gets a distribution that sums to exactly 1.0 (more deterministic
        # for the LLM consumer downstream).
        residual = round(1.0 - sum(weights.values()), 3)
        if residual != 0.0:
            max_emo = max(weights, key=weights.get)
            weights[max_emo] = round(weights[max_emo] + residual, 3)

    return EmotionDistribution(
        weights=weights,
        raw_counts={},
        total_rows=0,
    )