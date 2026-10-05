"""LLM 护栏规则测试。"""

from agent_os.llm.rules import ChatTurnRule, TokenCapRule, enforce_chat_turn


def test_chat_turn_6_force_close():
    """对话达到第 6 回合必须收尾，不能再问问题"""
    rule = ChatTurnRule(max_turns=6)
    # Turn 1-5: 允许继续
    for turn in range(1, 6):
        result = rule.check(turn=turn, last_response=f"回答 {turn}")
        assert result.allow_continue is True
    # Turn 6: 强制收尾
    result = rule.check(turn=6, last_response="回答 6")
    assert result.allow_continue is False
    assert "闲聊到此" in result.forced_closing or "下次再来" in result.forced_closing


def test_token_cap_block():
    rule = TokenCapRule(max_input_tokens=4000, max_output_tokens=300)
    result = rule.check(input_tokens=5000, output_tokens=100)
    assert result.exceeded is True
    assert "input_tokens" in result.reason
