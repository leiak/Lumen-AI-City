"""5 NPC 流式 prompt 模板测试。"""
import re

from agent_os.llm.prompts import get_npc_stream_prompt, STREAM_NPC_IDS


def test_stream_npc_ids_count():
    assert len(STREAM_NPC_IDS) == 5
    assert "npc_wang_boss_001" in STREAM_NPC_IDS
    assert "npc_grace_healer_001" in STREAM_NPC_IDS


def test_prompt_contains_emotion_tag_instruction():
    """每个 prompt 明确要求 XML emotion tag 输出。"""
    for npc_id in STREAM_NPC_IDS:
        prompt = get_npc_stream_prompt(npc_id, "你好", [])
        assert "<emotion=" in prompt
        assert "</emotion>" in prompt
        assert "<end>" in prompt


def test_prompt_contains_personality():
    """prompt 嵌入 NPC personality（OCEAN 描述）。"""
    prompt = get_npc_stream_prompt(
        "npc_wang_boss_001", "点菜", [{"role": "user", "content": "..."}],
    )
    assert "openness" in prompt or "性格" in prompt


def test_personalities_have_all_5_ocean_dimensions():
    """每个 prompt 必须含 5 个 OCEAN 维度（防 personality_desc 退化为 fallback）。"""
    for npc_id in STREAM_NPC_IDS:
        prompt = get_npc_stream_prompt(npc_id, "hi", [])
        for dim in ("openness=", "conscientiousness=", "extraversion=", "agreeableness=", "neuroticism="):
            assert dim in prompt, f"{npc_id} missing {dim}"


def test_personalities_are_unique_per_npc():
    """5 NPC 的 personality 描述必须各不相同（防退化到同一 fallback）。"""
    personalities = {}
    for npc_id in STREAM_NPC_IDS:
        prompt = get_npc_stream_prompt(npc_id, "hi", [])
        # 提取 personality_desc 段（在 "。" 之后的 OCEAN 数字前）
        m = re.search(r"你是.*?。(.+?)\n", prompt)
        assert m, f"{npc_id} personality_desc not extractable"
        personalities[npc_id] = m.group(1)
    assert len(set(personalities.values())) == 5, (
        f"NPCs share personality: {personalities}"
    )


def test_empty_context_renders_cleanly():
    """npc_context=[] 时 prompt 不含 history 段。"""
    prompt = get_npc_stream_prompt("npc_wang_boss_001", "你好", [])
    assert "<user>你好</user>" in prompt
    assert "<system>" in prompt
    assert prompt.endswith("Assistant:")
    # 无 history 时 system 与 user 之间应是单一换行
    assert "\n<user>你好</user>" in prompt


def test_multi_turn_context_renders():
    """npc_context 含多轮对话时全部渲染。"""
    ctx = [
        {"role": "user", "content": "q1"},
        {"role": "assistant", "content": "a1"},
    ]
    prompt = get_npc_stream_prompt("npc_wang_boss_001", "q2", ctx)
    assert "<user>q1</user>" in prompt
    assert "<assistant>a1</assistant>" in prompt
    assert "<user>q2</user>" in prompt


def test_non_dict_context_items_skipped():
    """npc_context 含非 dict 项静默跳过，不抛异常。"""
    ctx = [
        "raw string item",  # 非 dict
        {"role": "user", "content": "valid"},
        42,  # 非 dict
    ]
    prompt = get_npc_stream_prompt("npc_wang_boss_001", "hi", ctx)
    assert "<user>valid</user>" in prompt
    assert "raw string item" not in prompt
    assert prompt.endswith("Assistant:")