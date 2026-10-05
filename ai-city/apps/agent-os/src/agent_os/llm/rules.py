"""LLM 调用护栏规则 — chat_turns 强制收尾 + token cap。

背景：1.0 不接 LLM，所以这块代码全新。2.0 MVP 阶段接入 LLM 时，
每回合成本不可控，需要双重保险：

1. **ChatTurnRule**：spec §10 强制对话回合数（默认 6），
   超过后必须收尾，不能再继续提问。
2. **TokenCapRule**：限制单次调用的 input / output token 上限，
   超 cap 直接拒绝，避免一次调用吃光预算。

提供 `enforce_chat_turn()` 作为 helper，使用默认 6 回合收尾。
"""

from dataclasses import dataclass


@dataclass
class ChatTurnCheck:
    """ChatTurnRule.check() 的返回结果。"""

    allow_continue: bool
    forced_closing: str = ""


@dataclass
class TokenCapCheck:
    """TokenCapRule.check() 的返回结果。"""

    exceeded: bool
    reason: str = ""


class ChatTurnRule:
    """对话回合数强制收尾规则。"""

    def __init__(self, max_turns: int = 6):
        self.max_turns = max_turns

    def check(self, turn: int, last_response: str = "") -> ChatTurnCheck:
        if turn >= self.max_turns:
            return ChatTurnCheck(
                allow_continue=False,
                forced_closing="闲聊到此，下次再来吧！",
            )
        return ChatTurnCheck(allow_continue=True)


class TokenCapRule:
    """单次 LLM 调用的 token 上限规则。"""

    def __init__(self, max_input_tokens: int = 4000, max_output_tokens: int = 300):
        self.max_input_tokens = max_input_tokens
        self.max_output_tokens = max_output_tokens

    def check(self, input_tokens: int, output_tokens: int) -> TokenCapCheck:
        if input_tokens > self.max_input_tokens:
            return TokenCapCheck(
                exceeded=True,
                reason=f"input_tokens={input_tokens} > cap={self.max_input_tokens}",
            )
        if output_tokens > self.max_output_tokens:
            return TokenCapCheck(
                exceeded=True,
                reason=f"output_tokens={output_tokens} > cap={self.max_output_tokens}",
            )
        return TokenCapCheck(exceeded=False)


def enforce_chat_turn(turn: int, last_response: str = "") -> ChatTurnCheck:
    """默认 6 回合收尾的 helper。"""
    return ChatTurnRule().check(turn, last_response)
