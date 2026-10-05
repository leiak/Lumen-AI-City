"""2.0 dispatcher.say_with_llm 集成 LLM + 短路词 + chat_turn 测试。"""

import pytest

from agent_os.dispatcher import ActionDispatcher
from agent_os.llm.litellm_provider import LiteLLMProvider


@pytest.mark.asyncio
async def test_dispatcher_say_with_llm():
    """真实 LLM 调用 → 返回非空文本（需要 ANTHROPIC_API_KEY）。"""
    llm = LiteLLMProvider(model="claude-sonnet-4-6")
    dispatcher = ActionDispatcher(llm_client=llm)
    result = await dispatcher.say_with_llm(
        npc_id="npc_a_wang_boss",
        player_input="今天有什么好吃的？",
        npc_context=[],
        chat_turn=1,
    )
    assert result.npc_id == "npc_a_wang_boss"
    assert len(result.text) > 0
    assert result.trace_id != ""


@pytest.mark.asyncio
async def test_dispatcher_say_chat_turn_6_close():
    """chat_turn=6 → 强制收尾（无 LLM 调用）。"""
    llm = LiteLLMProvider(model="claude-sonnet-4-6")
    dispatcher = ActionDispatcher(llm_client=llm)
    result = await dispatcher.say_with_llm(
        npc_id="npc_a_wang_boss",
        player_input="再聊一个",
        npc_context=[],
        chat_turn=6,  # 第 6 回合
    )
    assert result.text == "闲聊到此，下次再来吧！"
    assert result.trace_id != ""