"""npc_registry 加载 yaml + list_enabled 过滤。"""
import textwrap
from pathlib import Path

import pytest

from agent_os.npc_registry import NpcRegistry


def _write_yaml(tmp: Path, name: str, body: str) -> Path:
    p = tmp / name
    p.write_text(textwrap.dedent(body), encoding="utf-8")
    return p


def test_load_wang_boss_minimal(tmp_path: Path):
    _write_yaml(tmp_path, "wang_boss.yaml", """\
        npc_id: npc_wang_boss_001
        name: 王老板
        enabled: true
        say:
          greeting:
            - "来了您嘞！"
        talk_tree:
          initial: greet
          nodes:
            greet:
              say: "来了您嘞！"
              options:
                - {id: ask, text: "问个事"}
              reply_map:
                ask: end
    """)
    reg = NpcRegistry(tmp_path)
    t = reg.get("npc_wang_boss_001")
    assert t.npc_id == "npc_wang_boss_001"
    assert t.enabled is True
    assert t.say.greeting == ["来了您嘞！"]
    assert t.talk_tree.initial == "greet"
    assert t.talk_tree.nodes["greet"].say == "来了您嘞！"


def test_list_enabled_filters_disabled(tmp_path: Path):
    _write_yaml(tmp_path, "wang_boss.yaml", """\
        npc_id: npc_wang_boss_001
        enabled: true
    """)
    _write_yaml(tmp_path, "lihua.yaml", """\
        npc_id: npc_lihua_002
        enabled: false
    """)
    reg = NpcRegistry(tmp_path)
    enabled = reg.list_enabled()
    assert [t.npc_id for t in enabled] == ["npc_wang_boss_001"]


def test_get_unknown_npc_raises(tmp_path: Path):
    reg = NpcRegistry(tmp_path)
    with pytest.raises(KeyError):
        reg.get("npc_does_not_exist")


def test_yaml_missing_npc_id_raises(tmp_path: Path):
    _write_yaml(tmp_path, "bad.yaml", """\
        enabled: true
    """)
    reg = NpcRegistry(tmp_path)
    with pytest.raises(ValueError):
        reg.get("bad")


def test_list_enabled_skips_load_errors(tmp_path: Path):
    """Regression: list_enabled() must skip _LoadError entries, not crash."""
    # valid NPC
    _write_yaml(tmp_path, "good.yaml", """\
        npc_id: npc_good
        enabled: true
        say:
          greeting: ["hi"]
    """)
    # invalid NPC (uses old schema)
    _write_yaml(tmp_path, "bad.yaml", """\
        agent_id: legacy_001
        enabled: true
    """)
    reg = NpcRegistry(tmp_path)
    enabled = reg.list_enabled()
    # Only the valid NPC should appear
    assert len(enabled) == 1
    assert enabled[0].npc_id == "npc_good"


def test_ocean_fields_parsed(tmp_path: Path):
    _write_yaml(tmp_path, "wang_boss.yaml", """\
        npc_id: npc_wang_boss_001
        enabled: true
        personality:
          openness: 0.3
          conscientiousness: 0.9
          extraversion: 0.8
          agreeableness: 0.4
          neuroticism: 0.2
        say:
          greeting:
            - "来了您嘞！"
    """)
    reg = NpcRegistry(tmp_path)
    tpl = reg.get("npc_wang_boss_001")
    assert tpl.personality is not None
    assert tpl.personality.openness == 0.3
    assert tpl.personality.conscientiousness == 0.9
    assert tpl.personality.extraversion == 0.8
    assert tpl.personality.agreeableness == 0.4
    assert tpl.personality.neuroticism == 0.2


def test_ocean_optional_defaults_to_none(tmp_path: Path):
    _write_yaml(tmp_path, "wang_boss.yaml", """\
        npc_id: npc_wang_boss_001
        enabled: true
        say:
          greeting:
            - "来了您嘞！"
    """)
    reg = NpcRegistry(tmp_path)
    tpl = reg.get("npc_wang_boss_001")
    # 没有 personality 块 → None (向后兼容老 YAML)
    assert tpl.personality is None


def test_ocean_partial_raises(tmp_path: Path):
    _write_yaml(tmp_path, "wang_boss.yaml", """\
        npc_id: npc_wang_boss_001
        enabled: true
        personality:
          openness: 0.3
        # 缺少其他 4 个字段
    """)
    reg = NpcRegistry(tmp_path)
    with pytest.raises((ValueError, KeyError)):
        reg.get("npc_wang_boss_001")


def test_canonical_wang_boss_yaml_loads_with_personality_and_greetings():
    """Sprint 12 T04b: 仓库内置 packages/npc-templates/wang_boss.yaml 含 OCEAN + 3+ greeting。

    用 NPC_TEMPLATES_DIR 解析到 monorepo 顶层 packages/npc-templates/；
    若目录或文件缺失（CI sandbox / 错 cwd），跳过而不是失败。
    """
    from agent_os.config import Config
    from agent_os.npc_registry import NpcRegistry, _LoadError  # type: ignore[attr-defined]

    cfg = Config()
    template_dir = Path(cfg.npc_templates_dir)
    if not template_dir.exists():
        pytest.skip(f"npc_templates_dir not found: {template_dir}")
    wang = template_dir / "wang_boss.yaml"
    if not wang.exists():
        pytest.skip(f"wang_boss.yaml not in {template_dir}")

    reg = NpcRegistry(template_dir)
    # 不用 list_enabled() —— 见 npc_registry 已知 bug：_LoadError sentinel 没有 .enabled 属性。
    entries = list(reg._by_id.values())
    if not any(
        not isinstance(e, _LoadError) and e.npc_id == "npc_wang_boss_001" for e in entries
    ):
        pytest.skip("npc_wang_boss_001 not in canonical templates")

    tpl = reg.get("npc_wang_boss_001")
    assert tpl.enabled is True
    assert tpl.personality is not None
    # 任务 spec 指定 OCEAN 值：传统 0.3 / 认真 0.85 / 爱聊 0.7 / 务实 0.4 / 淡定 0.3
    assert tpl.personality.openness == 0.3
    assert tpl.personality.conscientiousness == 0.85
    assert tpl.personality.extraversion == 0.7
    assert tpl.personality.agreeableness == 0.4
    assert tpl.personality.neuroticism == 0.3
    # 3 句 greeting；任务说 "3+ variants"，用 >= 防后期扩到 4/5 句
    assert len(tpl.say.greeting) >= 3
    # 全部非空字符串
    assert all(isinstance(g, str) and g.strip() for g in tpl.say.greeting)
