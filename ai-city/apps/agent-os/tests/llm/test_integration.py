# ai-city/apps/agent-os/tests/llm/test_integration.py
import os
import pytest
from agent_os.dispatcher import ActionDispatcher
from agent_os.llm.litellm_provider import LiteLLMProvider

@pytest.mark.skipif(not os.getenv("ANTHROPIC_API_KEY"), reason="no API key")
@pytest.mark.asyncio
async def test_end_to_end_wang_boss_say():
    llm = LiteLLMProvider(model="claude-sonnet-4-6")
    dispatcher = ActionDispatcher(llm_client=llm)
    # 第一轮
    r1 = await dispatcher.say_with_llm(
        npc_id="npc_a_wang_boss", player_input="老板，今天有什么好吃的？",
        npc_context=[], chat_turn=1,
    )
    assert "王老板" in r1.text or "老板" in r1.text or "酒馆" in r1.text
    # 第二轮（记忆召回 - stub 暂时空）
    r2 = await dispatcher.say_with_llm(
        npc_id="npc_a_wang_boss", player_input="那个红烧肉多少钱？",
        npc_context=[{"role": "user", "content": r1.text}],
        chat_turn=2,
    )
    assert len(r2.text) > 0