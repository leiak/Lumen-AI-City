# ai-city/apps/agent-os/src/agent_os/dispatcher.py
"""2.0 dispatcher: LLM-aware NPC say interface.

NOTE: This module is NEW for 2.0 Phase 1. The 1.0 dispatcher lives in
``action_dispatcher.py`` (Redis publish-only, no LLM) and is preserved
unchanged. This module defines the 2.0 ``ActionDispatcher`` contract that
generates context-aware NPC replies via LiteLLM (Phase 2 wires the real
LLM client; ``say_with_llm`` is the entry point that enforces short-circuit
词 + chat_turn 收尾 + NPC prompt + context 装配).

Stage 2 adds ``say_stream()`` — LLM token 流 → SentenceSplitter → 节拍事件流
→ Publisher.publish_beat/done 的全链路入口。

Spec ref:
- docs/superpowers/specs/2026-10-03-2.0-stage1-mvp-design.md §4.4
- docs/superpowers/specs/2026-10-05-2.0-stage2-stream-emotion-design.md §1/§2.1/§3.2
"""
from __future__ import annotations

import time
import uuid
from dataclasses import dataclass
from typing import AsyncIterator

from agent_os.llm.base import LLMRequest as _BaseLLMRequest  # for say_with_llm
from agent_os.llm.prompts import get_npc_prompt
from agent_os.llm.prompts import get_npc_stream_prompt
from agent_os.llm.rules import ChatTurnRule
from agent_os.llm.short_circuit import match_short_circuit
from agent_os.llm.types import LLMRequest
from agent_os.stream.emotion_validator import EmotionValidator
from agent_os.stream.sentence_splitter import SentenceSplitter


@dataclass
class DispatcherSayResult:
    npc_id: str
    text: str
    ts_ms: int
    trace_id: str


@dataclass
class DispatcherSayStreamEvent:
    """``say_stream()`` yield 的事件单元。

    - ``complete=False`` 表示一个句子的节拍事件（``text``/``emotion`` 有效）
    - ``complete=True``  表示流结束（``sentence_idx=None``, ``text=""``）
    """

    npc_id: str
    session_id: str
    sentence_idx: int | None  # None for done event
    text: str
    emotion: str
    complete: bool
    ts_ms: int
    trace_id: str


class ActionDispatcher:
    def __init__(
        self,
        llm_client=None,
        chat_rule: ChatTurnRule | None = None,
        publisher=None,
    ):
        self.llm_client = llm_client  # Phase 2: LiteLLM client
        self.chat_rule = chat_rule or ChatTurnRule(max_turns=6)
        self.publisher = publisher  # stage2: Redis 节拍发布器（可 None）

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
        req = _BaseLLMRequest(
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

    async def say_stream(
        self,
        npc_id: str,
        player_input: str,
        npc_context: list,
        session_id: str,
        trace_id: str | None,
    ) -> AsyncIterator[DispatcherSayStreamEvent]:
        """LLM token 流 → 句子节拍事件流 → Publisher.publish_beat/done。

        流程（spec §1）：
          1. 构造 LLMRequest（基于 ``get_npc_stream_prompt``）
          2. 调 ``llm_client.stream(req)`` 逐 chunk 喂给 SentenceSplitter
          3. 每个切出的 (text, raw_emotion) 经 EmotionValidator 规范化
          4. 构造 DispatcherSayStreamEvent → publisher.publish_beat → yield
          5. EOF：splitter.flush() 残余句 → beat；最后发 publish_done → yield done
          6. 异常：发 publish_done(complete=False) 并 raise（R_011 fallback）
        """
        if trace_id is None:
            trace_id = f"tr-{uuid.uuid4().hex[:12]}"

        splitter = SentenceSplitter()
        validator = EmotionValidator()
        sentence_idx = 0

        prompt = get_npc_stream_prompt(npc_id, player_input, npc_context)
        req = LLMRequest(prompt=prompt, model="claude-sonnet-4-6", max_tokens=512)

        async def _emit(
            text: str, raw_emotion: str, is_done: bool = False
        ) -> DispatcherSayStreamEvent:
            """内部 helper：构造事件 + publish + 返回。"""
            nonlocal sentence_idx
            validated = validator.validate(raw_emotion)
            event = DispatcherSayStreamEvent(
                npc_id=npc_id,
                session_id=session_id,
                sentence_idx=None if is_done else sentence_idx,
                text="" if is_done else text,
                emotion="neutral" if is_done else validated,
                complete=is_done,
                ts_ms=int(time.time() * 1000),
                trace_id=trace_id,
            )
            if is_done:
                await self.publisher.publish_done(
                    npc_id=npc_id,
                    session_id=session_id,
                    sentence_count=sentence_idx,
                    complete=True,
                    trace_id=trace_id,
                )
            else:
                await self.publisher.publish_beat(
                    npc_id=npc_id,
                    session_id=session_id,
                    sentence_idx=sentence_idx,
                    text=text,
                    emotion=validated,
                    trace_id=trace_id,
                )
                sentence_idx += 1
            return event

        try:
            async for token_chunk in self.llm_client.stream(req):
                # Each chunk is a dict like {"text": "...", "finish_reason": ...}
                chunk_text = token_chunk["text"]
                for text, raw_emotion in splitter.feed([chunk_text]):
                    yield await _emit(text, raw_emotion)

            # EOF：flush 残余（孤儿句 → emotion="neutral"）
            for text, raw_emotion in splitter.flush():
                yield await _emit(text, raw_emotion)

            # done 事件
            yield await _emit("", "", is_done=True)

        except Exception:
            # R_011 fallback：发 done(complete=False) 后让 caller 处理异常
            if self.publisher is not None:
                await self.publisher.publish_done(
                    npc_id=npc_id,
                    session_id=session_id,
                    sentence_count=sentence_idx,
                    complete=False,
                    trace_id=trace_id,
                )
            raise