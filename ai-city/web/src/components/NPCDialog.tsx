'use client';

/**
 * NPCDialog —— 浮动对话气泡（Sprint 12 T03c）
 *
 * 设计：
 *   - 监听 window 上的 `aicity:npc_dialogue` CustomEvent（由 ws-events.ts 桥发），
 *     detail 是完整信封（WsEnvelope<NpcDialoguePayload>）。
 *   - 区分两种语义（按 reply_to_choice_id === null 判）：
 *     - active say：NPC 主动说话，options 通常空，按 Esc 关闭
 *     - reply     ：NPC 回复某个玩家的选项，options 至少 1 条
 *   - 点击 option → api.postNpcTalk(npcId, choiceId, playerId) → 后端返 reply
 *     也会走 WS → 下一次 aicity:npc_dialogue 事件 → 重新渲染（或变成 showing-reply）
 *   - Esc 关闭；点遮罩外部关闭。
 *
 * min slice 约束：
 *   - npc 名称从 NPC_NAMES 静态表查；后续 T05+ 改从 /v1/npcs 拉。
 *   - 不做焦点陷阱 / ARIA 完整 a11y —— 1.0 简化，2.0 再补。
 */

import { useEffect, useRef, useState } from 'react';
import { api } from '@/lib/api';
import { NPC_DIALOGUE_EVENT } from '@/lib/ws-events';
import { useGameStore } from '@/store/game';
import type { NpcDialoguePayload, WsEnvelopeLike } from './NPCDialog.types';

/** npc_id → 显示名（Sprint 12 min slice 静态表；后续换 manifest / /v1/npcs） */
const NPC_NAMES: Record<string, string> = {
  npc_wang_boss_001: '王老板',
  npc_lihua_001: '李华',
};

export function NPCDialog() {
  const [payload, setPayload] = useState<NpcDialoguePayload | null>(null);
  const [busy, setBusy] = useState(false);
  const dialogRef = useRef<HTMLDivElement | null>(null);

  // 1) 监听 npc_dialogue 事件
  useEffect(() => {
    function onNpc(ev: Event) {
      const ce = ev as CustomEvent<WsEnvelopeLike>;
      if (ce.detail?.type !== 'npc_dialogue' || !ce.detail.payload) return;
      const p = ce.detail.payload;
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

  // 2) Esc 关闭
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') setPayload(null);
    }
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, []);

  if (!payload) return null;

  const name = NPC_NAMES[payload.npc_id] ?? payload.npc_id;
  const isReply = payload.reply_to_choice_id !== null;

  async function onOptionClick(choiceId: string) {
    if (!payload) return;
    setBusy(true);
    try {
      // player_id 优先取登录态（active say 的 payload.player_id 为空，直接透传会被
      // handler 的必填校验拦下）；未登录则退回 payload 值。
      const playerId = useGameStore.getState().playerId || payload.player_id || '';
      const reply = await api.postNpcTalk(payload.npc_id, choiceId, playerId);
      // Sprint 13: 用同步响应立刻渲染下一句（后端把响应体当唯一真源）。
      // WS 的 npc_dialogue 事件作为冗余；二者数据一致，先到先显示。
      setPayload(reply);
    } catch (e) {
      // 失败：保留当前 dialog，让用户重试或 Esc 关；这里只记日志
      console.error('[NPCDialog] postNpcTalk failed', e);
    } finally {
      setBusy(false);
    }
  }

  function onBackdropClick(e: React.MouseEvent<HTMLDivElement>) {
    // 只在点中 backdrop（不是 dialog 自身）时关
    if (e.target === e.currentTarget) setPayload(null);
  }

  return (
    <div
      role="dialog"
      aria-label="NPC 对话"
      onClick={onBackdropClick}
      className="fixed inset-0 z-40 flex items-end justify-center bg-black/30 pb-6"
    >
      <div
        ref={dialogRef}
        className="w-[28rem] max-w-[92vw] rounded-lg border border-slate-700 bg-slate-900/95 p-4 shadow-xl"
      >
        <div className="flex items-baseline justify-between gap-2">
          <div className="text-sm font-semibold text-emerald-400">{name}</div>
          <div className="text-xs text-slate-500">{payload.npc_id}</div>
        </div>

        <div className="mt-2 whitespace-pre-line text-base text-slate-100">
          {payload.say}
        </div>

        {payload.options.length > 0 ? (
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
      </div>
    </div>
  );
}
