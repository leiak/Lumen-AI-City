"""NpcTemplate.baseline_emotion_distribution 缓存测试 (Phase A.2)。

启动期一次性把 OCEAN → emotion baseline 派生并缓存到 NpcTemplate。
personality 缺失 → baseline=None（dispatcher 不注入人格段，向后兼容）。
"""
from __future__ import annotations

import textwrap
from pathlib import Path

import pytest

from agent_os.npc_registry import NpcRegistry
from agent_os.stream.emotion_validator import ALLOWED_EMOTIONS


def _write_yaml(tmp: Path, name: str, body: str) -> Path:
    p = tmp / name
    p.write_text(textwrap.dedent(body), encoding="utf-8")
    return p


# ---------------------------------------------------------------------------
# 1. personality 缺失 → baseline=None（向后兼容老 YAML）
# ---------------------------------------------------------------------------
def test_baseline_none_when_personality_missing(tmp_path: Path):
    """没有 personality 块 → baseline_emotion_distribution 必须为 None。"""
    _write_yaml(tmp_path, "wang_boss.yaml", """\
        npc_id: npc_wang_boss_001
        enabled: true
        say:
          greeting:
            - "来了您嘞！"
    """)
    reg = NpcRegistry(tmp_path)
    template = reg.get("npc_wang_boss_001")

    assert template.personality is None
    assert template.baseline_emotion_distribution is None


# ---------------------------------------------------------------------------
# 2. personality 存在 → baseline 非 None
# ---------------------------------------------------------------------------
def test_baseline_non_none_when_personality_set(tmp_path: Path):
    """personality 块存在 → baseline_emotion_distribution 必须为 EmotionDistribution。"""
    _write_yaml(tmp_path, "wang_boss.yaml", """\
        npc_id: npc_wang_boss_001
        enabled: true
        personality:
          openness: 0.3
          conscientiousness: 0.85
          extraversion: 0.7
          agreeableness: 0.4
          neuroticism: 0.3
        say:
          greeting:
            - "来了您嘞！"
    """)
    reg = NpcRegistry(tmp_path)
    template = reg.get("npc_wang_boss_001")

    assert template.personality is not None
    assert template.baseline_emotion_distribution is not None
    # baseline 必须长在 NpcTemplate 实例上（缓存而不是每次重新算）
    assert hasattr(template, "baseline_emotion_distribution")


# ---------------------------------------------------------------------------
# 3. weights 求和 ≈ 1.0
# ---------------------------------------------------------------------------
def test_baseline_weights_sum_to_one(tmp_path: Path):
    """baseline.weights 必须归一化（sum=1.0）。"""
    _write_yaml(tmp_path, "wang_boss.yaml", """\
        npc_id: npc_wang_boss_001
        enabled: true
        personality:
          openness: 0.6
          conscientiousness: 0.7
          extraversion: 0.8
          agreeableness: 0.5
          neuroticism: 0.4
    """)
    reg = NpcRegistry(tmp_path)
    template = reg.get("npc_wang_boss_001")

    assert template.baseline_emotion_distribution is not None
    total = sum(template.baseline_emotion_distribution.weights.values())
    assert total == pytest.approx(1.0, abs=0.001)


# ---------------------------------------------------------------------------
# 4. 包含全部 8 个 emotion
# ---------------------------------------------------------------------------
def test_baseline_includes_all_8_emotions(tmp_path: Path):
    """baseline.weights.keys() 必须覆盖全部 8 个 emotion。"""
    _write_yaml(tmp_path, "wang_boss.yaml", """\
        npc_id: npc_wang_boss_001
        enabled: true
        personality:
          openness: 0.5
          conscientiousness: 0.5
          extraversion: 0.5
          agreeableness: 0.5
          neuroticism: 0.5
    """)
    reg = NpcRegistry(tmp_path)
    template = reg.get("npc_wang_boss_001")

    assert template.baseline_emotion_distribution is not None
    keys = set(template.baseline_emotion_distribution.weights.keys())
    assert keys == set(ALLOWED_EMOTIONS)
    assert len(keys) == 8
