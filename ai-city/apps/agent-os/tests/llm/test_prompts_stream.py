"""5 NPC 流式 prompt 模板测试。"""
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