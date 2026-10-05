# ai-city/apps/agent-os/src/agent_os/dispatcher.py
"""2.0 dispatcher: LLM-aware NPC say interface.

NOTE: This module is NEW for 2.0 Phase 1. The 1.0 dispatcher lives in
``action_dispatcher.py`` (Redis publish-only, no LLM) and is preserved
unchanged. This module defines the 2.0 ``ActionDispatcher`` contract that
generates context-aware NPC replies via LiteLLM (Phase 2 wires the real
LLM client; ``say_with_llm`` is the entry point that enforces short-circuit
词 + chat_turn 收尾 + NPC prompt + context 装配).

Spec ref: docs/superpowers/specs/2026-10-03-2.0-stage1-mvp-design.md §4.4
"""
from __future__ import annotations

import time
import uuid
from dataclasses import dataclass

from agent_os.llm.base import LLMRequest
from agent_os.llm.prompts import get_npc_prompt
from agent_os.llm.rules import ChatTurnRule
from agent_os.llm.short_circuit import match_short_circuit


@dataclass
class DispatcherSayResult:
    npc_id: str
    text: str
    ts_ms: int
    trace_id: str


class ActionDispatcher:
    def __init__(self, llm_client=None, chat_rule: ChatTurnRule | None = None):
        self.llm_client = llm_client  # Phase 2: LiteLLM client
        self.chat_rule = chat_rule or ChatTurnRule(max_turns=6)

    async def say(self, npc_id: str, player_input: str, npc_context: list) -> DispatcherSayResult:
        # 1.0 Phase 1 占位：返回固定台词（BT path 由 action_dispatcher.py 保留）
        return DispatcherSayResult(
            npc_id=npc_id,
            text=f"[stub] 王老板回答：{player_input}",
            ts_ms=int(time.time() * 1000),
            trace_id="stub-trace",
        )

    async def say_with_llm(
        self,
        npc_id: str,
        player_input: str,
        npc_context: list,
        chat_turn: int,
    ) -> DispatcherSayResult:
        """LLM 路径：短路词 → chat_turn 强制收尾 → NPC prompt + context。

        优先级：
        1. 短路词（问候/告别）直接返回固定响应，零 LLM 成本
        2. chat_turn 达到 max_turns 强制收尾，避免无止境对话
        3. 否则用 NPC system prompt + 历史 context + 玩家输入调 LLM
        """
        trace_id = f"tr-{uuid.uuid4().hex[:12]}"

        # 1) 短路词优先（无 LLM 成本）
        short_resp = match_short_circuit(player_input)
        if short_resp is not None:
            return DispatcherSayResult(
                npc_id=npc_id,
                text=short_resp,
                ts_ms=int(time.time() * 1000),
                trace_id=trace_id,
            )

        # 2) chat_turn 强制收尾（默认 6 回合）
        turn_check = self.chat_rule.check(turn=chat_turn, last_response="")
        if not turn_check.allow_continue:
            return DispatcherSayResult(
                npc_id=npc_id,
                text=turn_check.forced_closing,
                ts_ms=int(time.time() * 1000),
                trace_id=trace_id,
            )

        # 3) 调 LLM（NPC 人格 + 历史 context + 玩家输入）
        if self.llm_client is None:
            # 无 LLM 客户端 → 降级到 stub，避免 NPE
            return DispatcherSayResult(
                npc_id=npc_id,
                text=f"[stub-no-llm] {get_npc_prompt(npc_id).split(chr(10))[0]}: {player_input}",
                ts_ms=int(time.time() * 1000),
                trace_id=trace_id,
            )

        system_prompt = get_npc_prompt(npc_id)
        messages = list(npc_context) + [{"role": "user", "content": player_input}]
        req = LLMRequest(
            system_prompt=system_prompt,
            messages=messages,
            max_tokens=100,
            temperature=0.7,
        )
        resp = await self.llm_client.complete(req)
        return DispatcherSayResult(
            npc_id=npc_id,
            text=resp.text,
            ts_ms=int(time.time() * 1000),
            trace_id=trace_id,
        )