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

import asyncio
import logging
import os
import time
import uuid
from collections.abc import AsyncIterator
from dataclasses import dataclass
from typing import TYPE_CHECKING, Any

from agent_os.errors import R015SessionNotFound
from agent_os.llm.base import LLMRequest as _BaseLLMRequest  # for say_with_llm
from agent_os.llm.prompts import get_npc_prompt, get_npc_stream_prompt
from agent_os.llm.rules import ChatTurnRule
from agent_os.llm.short_circuit import match_short_circuit
from agent_os.llm.types import LLMRequest
from agent_os.stream.emotion_validator import EmotionValidator
from agent_os.stream.sentence_splitter import SentenceSplitter
from agent_os.stream.session_store import SessionStore

if TYPE_CHECKING:
    from agent_os.bt.state import BTState

_logger = logging.getLogger(__name__)

# ---- W4.2: dispatcher post-hook → npc_sell_to_player -----------------------
# After ``say_stream()`` finishes the LLM token stream, an opt-in caller
# (or env-driven default) can queue a fire-and-forget BT action
# (``npc_sell_to_player``) via ``asyncio.create_task`` — the streaming
# response itself is not blocked by the upstream HTTP call.

# Environment kill-switch / default opt-in. Callers may also pass
# ``enable_bt_post_hook=True`` explicitly to ``say_stream()`` regardless of env.
BT_POST_HOOK_ENABLED: bool = (
    os.environ.get("BT_POST_HOOK_ENABLED", "false").lower() == "true"
)


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


async def _run_post_hook(action_name: str, kwargs: dict[str, Any], bt_state: BTState) -> None:
    """Background task — fire-and-forget BT action after ``say_stream()`` ends.

    Looks the action up in :data:`agent_os.bt.actions.ACTION_REGISTRY` and
    invokes it synchronously with the queued ``kwargs``. The sync action is
    run in a worker thread via :func:`asyncio.to_thread` so its blocking
    ``httpx.Client.post`` call (up to ~5s on the economy endpoint) does not
    stall the event loop. All exceptions are swallowed + logged at WARNING —
    the post-hook must never propagate into the streaming caller.
    """
    # Local import keeps the dependency optional (BT layer is required for the
    # 2.0 dispatcher, but tests that never trigger the post-hook avoid the cost).
    from agent_os.bt.actions import ACTION_REGISTRY

    action_fn = ACTION_REGISTRY.get(action_name)
    if action_fn is None:
        _logger.warning("post_hook: unknown action %s", action_name)
        return
    try:
        # Wrap sync action in to_thread so concurrent hooks don't serialize
        # on the event loop. Without this, a slow economy call (5s timeout)
        # would block every other coroutine in the loop.
        status = await asyncio.to_thread(action_fn, state=bt_state, **kwargs)
        _logger.info(
            "post_hook.%s: status=%s product=%s",
            action_name,
            status.name,
            kwargs.get("product_id"),
        )
    except Exception as e:  # noqa: BLE001 — defensive, post-hook is best-effort
        _logger.warning("post_hook.%s failed: %s", action_name, e)


def schedule_post_hook(
    bt_state: BTState, enabled: bool = False,
) -> asyncio.Task[None] | None:
    """Schedule the post-stream BT action as fire-and-forget.

    Returns ``None`` when ``enabled`` is ``False`` or no ``pending_purchase``
    is queued on ``bt_state``. The caller (:meth:`ActionDispatcher.say_stream`)
    is responsible for resolving the effective enabled state — the explicit
    ``enable_bt_post_hook`` kwarg **overrides** the env kill-switch (kwarg
    ``True`` + env ``False`` → still runs), while kwarg ``False`` defers to
    the env. The returned :class:`asyncio.Task` is intentionally not awaited —
    callers (i.e. the streaming endpoint) should not block on the upstream
    economy HTTP call.

    Args:
        bt_state: A :class:`BTState` whose ``pending_purchase`` carries the
            kwargs (e.g. ``{"product_id": 42, "currency": "gold"}``) for the
            ``npc_sell_to_player`` action.
        enabled: Pre-resolved (kwarg OR env) flag. ``False`` short-circuits
            without touching the BT layer.
    """
    if not enabled:
        return None
    if not getattr(bt_state, "pending_purchase", None):
        return None
    pending = dict(bt_state.pending_purchase)
    return asyncio.create_task(
        _run_post_hook("npc_sell_to_player", pending, bt_state),
    )


class ActionDispatcher:
    def __init__(
        self,
        llm_client=None,
        chat_rule: ChatTurnRule | None = None,
        publisher=None,
        session_store: SessionStore | None = None,
        memory_writer=None,
        emotion_repo=None,
        emotion_settings=None,
        npc_registry=None,  # A.4: NpcRegistry for OCEAN baseline lookup
    ):
        self.llm_client = llm_client  # Phase 2: LiteLLM client
        self.chat_rule = chat_rule or ChatTurnRule(max_turns=6)
        self.publisher = publisher  # stage2: Redis 节拍发布器（可 None）
        self.session_store = session_store  # stage2: 重连补帧 buffer（可 None → 不 wire）
        # B2: optional best-effort emotion persistence hook; None = disabled
        self.memory_writer = memory_writer
        # B2-T08: optional EmotionRepository + settings for prompt-time distribution
        # injection. None = no injection (pre-B2 behavior).
        self.emotion_repo = emotion_repo
        self.emotion_settings = emotion_settings
        # A.4: optional NpcRegistry for OCEAN baseline lookup at prompt build time.
        # None = no baseline injection (pre-A.4 behavior). The kill switch
        # OCEAN_BIAS_ENABLED (EmotionSettings.ocean_bias_enabled) also gates this.
        self.npc_registry = npc_registry
        # B2: per-session emotion collection (reset at start of say_stream)
        self._emotions_emitted: list[str] = []

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
        model: str = "claude-haiku-4-5",
        max_tokens: int = 200,
        player_id: str | None = None,
        enable_bt_post_hook: bool = False,
        pending_purchase: dict[str, Any] | None = None,
    ) -> AsyncIterator[DispatcherSayStreamEvent]:
        """LLM token 流 → 句子节拍事件流 → Publisher.publish_beat/done。

        流程（spec §1）：
          1. 构造 LLMRequest（基于 ``get_npc_stream_prompt``）
          2. 调 ``llm_client.stream(req)`` 逐 chunk 喂给 SentenceSplitter
          3. 每个切出的 (text, raw_emotion) 经 EmotionValidator 规范化
          4. 构造 DispatcherSayStreamEvent → publisher.publish_beat → yield
             → （如注入 session_store）session_store.append
          5. EOF：splitter.flush() 残余句 → beat；最后发 publish_done → yield done
             → session_store.mark_done(complete=True)
             → (B2) MemoryWriter.write_emotions (best-effort)
          6. 异常：发 publish_done(complete=False) + session_store.mark_done(complete=False)
             并 raise（R_011 fallback）

        SessionStore wire（如 ``ActionDispatcher(..., session_store=...)`` 注入）：
          - caller 传的 ``session_id`` 不在 store 里时，dispatcher 自动 mint 新 sid
            并 override 该 sid 用于 publisher + store（保持唯一 id 用于重连补帧）
          - 已存在的 sid 沿用 caller 提供的（典型：先 create 再 stream）

        Args:
            model: LLM model id forwarded to ``LLMRequest`` (default ``claude-haiku-4-5``
                for cost — tests pass ``claude-haiku-4-5``; prod callers may upgrade to
                sonnet for richer responses).
            max_tokens: Hard cap on LLM output tokens (default 200).
            player_id: B2 optional player id forwarded to ``MemoryWriter.write_emotions``
                for emotion persistence. ``None`` → falls back to ``session_id`` as
                proxy (session-scoped persistence only).
            enable_bt_post_hook: W4.2 opt-in. When ``True`` (default ``False``),
                enables the post-hook for this call **regardless of**
                :data:`BT_POST_HOOK_ENABLED` env (kwarg explicitly overrides
                env-off). When ``False``, defers to env — env-on enables the
                hook, env-off keeps it disabled. Requires a ``pending_purchase``
                to do anything.
            pending_purchase: W4.2 kwargs (e.g. ``{"product_id": 42, "currency":
                "gold"}``) consumed by the post-hook action. Ignored when
                ``enable_bt_post_hook`` is ``False`` and the env var is off.
        """
        # B2: reset per-session emotion collection so dispatcher (singleton) doesn't
        # leak emotions across say_stream() invocations.
        self._emotions_emitted = []

        # W4.2: decide up front whether the post-stream BT action should run.
        # Either the explicit kwarg or the env kill-switch enables it; a
        # pending_purchase is required for the hook to do anything.
        _post_hook_enabled = bool(enable_bt_post_hook or BT_POST_HOOK_ENABLED)
        _bt_state: BTState | None = None
        if _post_hook_enabled and pending_purchase:
            from agent_os.bt.state import BTState

            _bt_state = BTState(player_id=player_id, pending_purchase=dict(pending_purchase))

        if trace_id is None:
            trace_id = f"tr-{uuid.uuid4().hex[:12]}"

        # B2: when caller didn't pass player_id, fall back to session-scoped proxy.
        # write_emotions requires non-None player_id (PG schema NOT NULL); session_id
        # uniquely identifies the conversation so this is a safe proxy for the
        # emotion persistence use case (no global player aggregation unless caller
        # passes the real player_id).
        effective_player_id = player_id or session_id

        # Wire SessionStore（如注入）：保证 session 存在；用于断线重连补帧
        store = self.session_store
        if store is not None:
            try:
                store._require(session_id)  # 已存在 → 沿用 caller 提供的 sid
                store_sid = session_id
            except R015SessionNotFound:
                # caller 传的 sid 不在 store 里（典型：测试 stub 或新流未创建）
                # mint 一个新的并 override
                store_sid = store.create(npc_id)
        else:
            store_sid = session_id

        splitter = SentenceSplitter()
        validator = EmotionValidator()
        sentence_idx = 0

        # B2-T08: fetch emotion distributions for prompt injection (best-effort).
        # Failure → log warning + fall back to None (pre-B2 prompt shape).
        # Respects EMOTION_INJECT_ENABLED kill switch (settings.inject_enabled).
        # Aggregation latency + row counts logged at INFO (spec §9 #5).
        recent_dist = None
        global_dist = None
        inject_enabled = bool(
            self.emotion_settings and self.emotion_settings.inject_enabled,
        )
        if self.emotion_repo is not None and inject_enabled:
            t0 = time.monotonic()
            try:
                recent_dist = await self.emotion_repo.fetch_player_distribution(
                    npc_id, effective_player_id,
                )
            except Exception as e:  # noqa: BLE001
                _logger.warning(
                    "emotion aggregate (player) failed (npc=%s session=%s): %s "
                    "— falling back to no-injection",
                    npc_id, store_sid, e,
                )
            try:
                global_dist = await self.emotion_repo.fetch_global_distribution(npc_id)
            except Exception as e:  # noqa: BLE001
                _logger.warning(
                    "emotion aggregate (global) failed (npc=%s session=%s): %s "
                    "— falling back to no-injection",
                    npc_id, store_sid, e,
                )
            elapsed_ms = (time.monotonic() - t0) * 1000.0
            _logger.info(
                "emotion aggregate npc=%s latency_ms=%.3f recent_rows=%d "
                "global_rows=%d repo=%s",
                npc_id, elapsed_ms,
                recent_dist.total_rows if recent_dist else 0,
                global_dist.total_rows if global_dist else 0,
                type(self.emotion_repo).__name__,
            )

        # A.4: fetch OCEAN baseline from registry (best-effort). Failure → log warning
        # + fall back to None (no-baseline prompt shape). Respects
        # OCEAN_BIAS_ENABLED kill switch.
        baseline_dist = None
        ocean_bias_enabled = bool(
            self.emotion_settings and self.emotion_settings.ocean_bias_enabled,
        )
        if ocean_bias_enabled and self.npc_registry is not None:
            try:
                npc_template = self.npc_registry.get(npc_id)
                baseline_dist = npc_template.baseline_emotion_distribution
            except (KeyError, ValueError) as e:  # unknown npc_id or bad YAML
                _logger.warning(
                    "OCEAN baseline fetch failed (npc=%s session=%s): %s "
                    "— falling back to no-baseline",
                    npc_id, store_sid, e,
                )
            except Exception as e:  # noqa: BLE001 — best-effort
                _logger.warning(
                    "OCEAN baseline unexpected error (npc=%s session=%s): %s",
                    npc_id, store_sid, e,
                )

        prompt = get_npc_stream_prompt(
            npc_id,
            player_input,
            npc_context,
            recent_distribution=recent_dist,
            global_distribution=global_dist,
            baseline_distribution=baseline_dist,  # A.4
        )
        req = LLMRequest(prompt=prompt, model=model, max_tokens=max_tokens)

        async def _emit(
            text: str, raw_emotion: str, is_done: bool = False
        ) -> DispatcherSayStreamEvent:
            """内部 helper：构造事件 + publish + 返回。"""
            nonlocal sentence_idx
            validated = validator.validate(raw_emotion)
            event = DispatcherSayStreamEvent(
                npc_id=npc_id,
                session_id=store_sid,
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
                    session_id=store_sid,
                    sentence_count=sentence_idx,
                    complete=True,
                    trace_id=trace_id,
                )
                # SessionStore wire：done → mark_done(complete=True)
                if store is not None:
                    store.mark_done(store_sid, complete=True)
                # B2: best-effort emotion persist on successful completion.
                # write_emotions itself swallows PG errors, but we wrap defensively
                # so any unexpected exception here never breaks the caller stream.
                if self.memory_writer is not None and self._emotions_emitted:
                    try:
                        await self.memory_writer.write_emotions(
                            npc_id=npc_id,
                            player_id=effective_player_id,
                            session_id=store_sid,
                            emotions=list(self._emotions_emitted),
                        )
                    except Exception as e:  # pragma: no cover - defensive
                        _logger.warning(
                            "dispatcher: emotion persist failed (npc=%s session=%s): %s",
                            npc_id, store_sid, e,
                        )
            else:
                # SessionStore wire：每句 beat → append FIRST
                # (buffer is source of truth；publish 之前完成 append，
                # 避免重连窗口期 GET /buffer 漏 beat — fix review issue #2)
                if store is not None:
                    store.append(
                        store_sid,
                        sentence_idx=sentence_idx,
                        text=text,
                        emotion=validated,
                    )
                # 然后 publish_beat（下游 Redis 通知）
                await self.publisher.publish_beat(
                    npc_id=npc_id,
                    session_id=store_sid,
                    sentence_idx=sentence_idx,
                    text=text,
                    emotion=validated,
                    trace_id=trace_id,
                )
                # B2: collect this beat's validated emotion for write_emotions
                if validated:
                    self._emotions_emitted.append(validated)
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
                    session_id=store_sid,
                    sentence_count=sentence_idx,
                    complete=False,
                    trace_id=trace_id,
                )
            # SessionStore wire：异常退出也 mark_done(complete=False)，
            # 让重连端点能区分"流正常结束" vs "流异常结束"。
            if store is not None:
                try:
                    store.mark_done(store_sid, complete=False)
                except R015SessionNotFound:
                    # session 已被 TTL 清掉 — best-effort 忽略
                    pass
            # B2: 清空情绪收集（异常时不持久化）
            self._emotions_emitted = []
            raise
        finally:
            # B2: ensure per-session emotion buffer is reset after stream ends
            # (success or failure), so the next say_stream() call starts fresh.
            self._emotions_emitted = []
            # W4.2: schedule the BT post-hook AFTER the stream yields the
            # done event (or after an exception is raised above). Running in
            # ``finally`` ensures the hook fires regardless of upstream
            # outcome — caller still receives the generator's normal
            # termination / propagated exception. The task is fire-and-forget
            # so the caller never blocks on the upstream economy HTTP call.
            # Gating: kwarg (enable_bt_post_hook) OR env (BT_POST_HOOK_ENABLED)
            # was resolved into ``_post_hook_enabled`` at the top of this method.
            if _bt_state is not None:
                schedule_post_hook(_bt_state, enabled=_post_hook_enabled)