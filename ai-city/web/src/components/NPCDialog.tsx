'use client';

/**
 * NPCDialog —— 浮动对话气泡（Sprint 12 T03c + 2.0 阶段 2 T18）
 *
 * 2.0 阶段 2 之前：监听 window 上 `aicity:npc_dialogue` CustomEvent，
 * 渲染完整 say + 选项；点击 option 调 api.postNpcTalk(npcId, choiceId, playerId)。
 *
 * 2.0 阶段 2 起：再订阅 `aicity:npc_say_stream` 频道（同一窗口，不同事件名），
 * 把节拍 (npc_say_stream) 累加成句子列表，逐句渲染。结束标记
 * (npc_say_stream_done) 触发 emotion chip 淡出 + UI 收尾。
 *
 * 渲染优先级：
 *   1. 若任一会话已有任何节拍 → 渲染 sentence-by-sentence 列表 + 头像旁
 *      emotion emoji overlay（取代旧的单一 say 文本）
 *   2. 否则（npc_dialogue 旧路径 / 流式断流 fallback）→ 渲染 payload.say
 *      + options（保留选项点击交互，向后兼容）
 *
 * 去重 / 重连补帧：
 *   - 节拍按 (session_id, sentence_idx) 复合键去重 —— 同一会话内 sentence_idx
 *     永不重复，断连重连时 ws-gateway 通过 session_store 把已发句子补帧回来，
 *     客户端按 sentence_idx 覆盖写，不会出现重复行
 *   - 不同 session_id 视作新会话（旧 beats 不清空 —— 同一 NPC 切换 topic 时
 *     玩家能回看上一段）。如需主动清空，调用方发 npc_say_stream_done + 后端
 *     开新 session 即可（前端不需要额外 reset API）
 *
 * emotion 白名单兜底 —— 走 emojiForEmotion()；非 8 类 emotion 一律回退
 * neutral emoji，避免后端 EmotionValidator 漏网时渲染空白 / 报错。
 *
 * min slice 约束：
 *   - npc 名称从 NPC_NAMES 静态表查；后续 T05+ 改从 /v1/npcs 拉
 *   - emotion chip 用 emoji 而非 SVG（spec §6 风险：避免引入图片资源）
 *   - 不做焦点陷阱 / ARIA 完整 a11y —— 1.0 简化，2.0 再补
 */

import { useEffect, useMemo, useRef, useState } from 'react';
import { api } from '@/lib/api';
import {
  NPC_DIALOGUE_EVENT,
  NPC_EMOTION_EMOJI,
  NPC_SAY_STREAM_EVENT,
} from '@/lib/ws-events';
import type { NpcEmotion } from '@/lib/ws-events';
import { useGameStore } from '@/store/game';
import type {
  NpcDialoguePayload,
  NpcSayStreamBeat,
  NpcSayStreamDone,
  WsEnvelopeLike,
} from './NPCDialog.types';

/** npc_id → 显示名（Sprint 12 min slice 静态表；后续换 manifest / /v1/npcs） */
const NPC_NAMES: Record<string, string> = {
  npc_wang_boss_001: '王老板',
  npc_lihua_001: '李华',
};

/** 单句节拍 —— session 内 sentence_idx 对应的渲染单元 */
interface BeatEntry {
  text: string;
  emotion: string;
}

/** 单个会话的缓冲：npc_id（首帧尾注入）+ sentence_idx → beat */
interface SessionBuffer {
  npc_id: string;
  beats: Record<number, BeatEntry>;
}

/**
 * 防御性 emotion 兜底：不在 8 类白名单内一律返回 neutral。EmotionValidator
 * 应已在 agent-os 端降级，但前端再防一层以防 wire 上出现未预期值（spec §3
 * 风险 R_013 的前端镜像处理）。
 */
function emojiForEmotion(raw: string): string {
  if (raw in NPC_EMOTION_EMOJI) {
    return NPC_EMOTION_EMOJI[raw as NpcEmotion];
  }
  return NPC_EMOTION_EMOJI.neutral;
}

export function NPCDialog() {
  // 旧 batch (npc_dialogue) —— 保留作 fallback
  const [payload, setPayload] = useState<NpcDialoguePayload | null>(null);
  const [busy, setBusy] = useState(false);
  const dialogRef = useRef<HTMLDivElement | null>(null);

  // 2.0 阶段 2 新增：sentence-by-sentence 流式状态
  /** 按 session_id 分组的句子缓冲 */
  const [streamBySession, setStreamBySession] = useState<
    Record<string, SessionBuffer>
  >({});
  /** 当前有流式 beat 的 session（最近一个 update 选择的会话） */
  const [activeSessionId, setActiveSessionId] = useState<string | null>(null);
  /** 头像旁显示的 emoji —— 跟随最新 beat 的 emotion；done 后淡出回 neutral */
  const [currentEmotion, setCurrentEmotion] = useState<string>('neutral');
  /** done 事件触发后置 true → emotion chip 淡出 */
  const [streamFinalized, setStreamFinalized] = useState(false);

  // 1) 监听 npc_dialogue（batch fallback —— 保留向后兼容）
  useEffect(() => {
    function onNpc(ev: Event) {
      const ce = ev as CustomEvent<WsEnvelopeLike>;
      if (ce.detail?.type !== 'npc_dialogue' || !ce.detail.payload) return;
      const p = ce.detail.payload as NpcDialoguePayload;
      if (!('say' in p)) return;
      // Sprint 13：专属台词（welcome / reply 带非空 player_id）只渲染给目标玩家；
      // 主动广播（player_id==""）或未登录（无 myId 可比对）时对所有人显示。
      const myId = useGameStore.getState().playerId;
      if (p.player_id && myId && p.player_id !== myId) return;
      setPayload(p);
    }
    window.addEventListener(NPC_DIALOGUE_EVENT, onNpc as EventListener);
    return () =>
      window.removeEventListener(NPC_DIALOGUE_EVENT, onNpc as EventListener);
  }, []);

  // 2) 监听 npc_say_stream（2.0 阶段 2 新增）
  useEffect(() => {
    function onStream(ev: Event) {
      const ce = ev as CustomEvent<WsEnvelopeLike>;
      const detail = ce.detail;
      if (!detail || detail.type !== 'npc_say_stream') return;
      const inner = detail.payload as
        | NpcSayStreamBeat
        | NpcSayStreamDone
        | undefined;
      if (!inner) return;

      // --- 节拍包（单句）----
      if (inner.type === 'npc_say_stream') {
        const beat = inner as NpcSayStreamBeat;
        const sid = beat.session_id;
        setActiveSessionId(sid);
        setStreamFinalized(false);
        setStreamBySession((prev) => {
          const existing = prev[sid];
          // 复合键 (session_id, sentence_idx) 去重 —— 重连补帧时同 idx 覆盖写
          const nextBeats: Record<number, BeatEntry> = {
            ...(existing?.beats ?? {}),
            [beat.sentence_idx]: { text: beat.text, emotion: beat.emotion },
          };
          return {
            ...prev,
            [sid]: {
              npc_id: existing?.npc_id ?? beat.npc_id,
              beats: nextBeats,
            },
          };
        });
        // emotion 跟随最新 beat（spec §6：句末切换 → 头像旁 emoji）
        setCurrentEmotion(beat.emotion);
        return;
      }

      // --- 结束标记 ----
      if (inner.type === 'npc_say_stream_done') {
        const done = inner as NpcSayStreamDone;
        setStreamFinalized(true);
        // done 到达后 ~1.5s 把 emotion 还原 neutral（chip 淡出动画窗口）
        window.setTimeout(() => {
          setCurrentEmotion('neutral');
        }, 1500);
        // 不清空 beats；后续流式句点自然覆盖。旧会话保留作历史回放。
      }
    }
    window.addEventListener(NPC_SAY_STREAM_EVENT, onStream as EventListener);
    return () =>
      window.removeEventListener(NPC_SAY_STREAM_EVENT, onStream as EventListener);
  }, []);

  // 3) Esc 关闭（仅清空 npc_dialogue 路径；stream 路径自然累积）
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') setPayload(null);
    }
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, []);

  // 当前展示的会话与 beats 列表（按 sentence_idx 升序）
  const activeBuffer = activeSessionId
    ? streamBySession[activeSessionId]
    : undefined;
  const activeBeats = useMemo(() => {
    if (!activeBuffer) return [];
    return Object.keys(activeBuffer.beats)
      .map((k) => Number(k))
      .sort((a, b) => a - b)
      .map((idx) => ({ idx, ...activeBuffer.beats[idx] }));
  }, [activeBuffer]);

  const showStream = activeBeats.length > 0;
  const streamNpcId = activeBuffer?.npc_id ?? '';
  const finalNpcId = streamNpcId || payload?.npc_id || '';

  // 都没东西 → 不渲染
  if (!payload && !showStream) return null;

  const name = NPC_NAMES[finalNpcId] ?? finalNpcId;
  const isReply = payload?.reply_to_choice_id != null;

  async function onOptionClick(choiceId: string) {
    if (!payload) return;
    setBusy(true);
    try {
      const playerId = useGameStore.getState().playerId || payload.player_id || '';
      const reply = await api.postNpcTalk(payload.npc_id, choiceId, playerId);
      setPayload(reply);
    } catch (e) {
      console.error('[NPCDialog] postNpcTalk failed', e);
    } finally {
      setBusy(false);
    }
  }

  function onBackdropClick(e: React.MouseEvent<HTMLDivElement>) {
    if (e.target === e.currentTarget) setPayload(null);
  }

  const emotionEmoji = emojiForEmotion(currentEmotion);

  return (
    <div
      role="dialog"
      aria-label="NPC 对话"
      data-testid="npc-dialog"
      onClick={onBackdropClick}
      className="fixed inset-0 z-40 flex items-end justify-center bg-black/30 pb-6"
    >
      <div
        ref={dialogRef}
        className="w-[28rem] max-w-[92vw] rounded-lg border border-slate-700 bg-slate-900/95 p-4 shadow-xl"
      >
        {/* 头部：NPC 名称 + 头像旁 emotion overlay */}
        <div className="flex items-center justify-between gap-3">
          <div className="flex items-center gap-2">
            <div className="relative inline-flex h-9 w-9 items-center justify-center rounded-full bg-slate-700 text-base">
              {/* 占位头像圆盘 —— 后续接 /v1/npcs/{id}/avatar.png */}
              <span aria-hidden>
                {finalNpcId ? finalNpcId.slice(-2) : '?'}
              </span>
              {showStream && (
                <span
                  className="emotion-overlay absolute -bottom-1 -right-1 flex h-5 w-5 items-center justify-center rounded-full border border-slate-900 bg-slate-800 text-sm leading-none"
                  data-emotion={currentEmotion}
                  style={{
                    transition: 'opacity 200ms ease-out',
                    opacity: streamFinalized ? 0 : 1,
                  }}
                  aria-label={`emotion ${currentEmotion}`}
                >
                  {emotionEmoji}
                </span>
              )}
            </div>
            <div className="flex flex-col">
              <div className="text-sm font-semibold text-emerald-400">
                {name}
              </div>
              <div className="text-xs text-slate-500">{finalNpcId}</div>
            </div>
          </div>
          <div className="text-xs text-slate-500">
            {showStream
              ? streamFinalized
                ? '流式对话已结束'
                : '流式对话中…'
              : ''}
          </div>
        </div>

        {/* 内容：流式 beats 优先；fallback 到 npc_dialogue.say + options */}
        {showStream ? (
          <div className="npc-sentences mt-3 max-h-72 space-y-2 overflow-y-auto pr-1">
            {activeBeats.map((b) => (
              <p
                key={`${activeSessionId}:${b.idx}`}
                className="sentence rounded border border-slate-800 bg-slate-800/60 px-3 py-2 text-base text-slate-100"
                style={{
                  animation: 'npc-sentence-fade-in 200ms ease-out',
                }}
                data-sentence-idx={b.idx}
              >
                <span
                  className="mr-2 inline-block text-sm text-slate-400"
                  aria-hidden
                >
                  {emojiForEmotion(b.emotion)}
                </span>
                {b.text}
              </p>
            ))}
          </div>
        ) : (
          <>
            <div className="mt-2 whitespace-pre-line text-base text-slate-100">
              {payload?.say}
            </div>
            {payload && payload.options.length > 0 ? (
              <div className="mt-3 flex flex-col gap-2">
                {payload.options.map((opt) => (
                  <button
                    key={opt.id}
                    type="button"
                    disabled={busy}
                    onClick={() => onOptionClick(opt.id)}
                    className="rounded border border-slate-600 bg-slate-800 px-3 py-2 text-left text-sm text-slate-100 hover:bg-slate-700 disabled:opacity-50"
                  >
                    {opt.text}
                  </button>
                ))}
              </div>
            ) : (
              <div className="mt-3 text-xs text-slate-400">
                （{isReply ? '对话已结束' : 'NPC 主动招呼'} · 按 Esc 关闭）
              </div>
            )}
          </>
        )}
      </div>
    </div>
  );
}