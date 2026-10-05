/**
 * WS 推送 → 应用状态的桥（Sprint 9 + Sprint 12）。
 *
 * 取代 WorldMap 原来的 3s 轮询：ws-gateway 收到 world-engine 的
 * `aicity:player:moved` 后立刻扇出，这里把信封分派成：
 *   - 自己的 move echo   → store.setPosition 校准（服务端权威坐标）
 *   - 任何玩家的 move    → dispatch `aicity:player_moved` CustomEvent，
 *                          WorldMap 收到后 debounce 重拉 /v1/tiles
 *
 * Sprint 12 新增 npc_dialogue 分派（来自 ws-gateway 的 agent-os NPC 对话流）：
 *   - dispatch `aicity:npc_dialogue` CustomEvent，NPCDialog 组件订阅；
 *     detail 是完整信封（envelope），消费方按 `e.detail.payload.{npc_id,say,...}` 取值
 *   - 不进 zustand：对话是 ephemeral 推送流，组件本地 useState 更合适
 *     （同一 NPC 多条对话重叠、Sprint 13 之后才考虑进 store 做历史回放）
 *
 * 为什么 player_moved 还要重拉 tiles：信封只带一个玩家的新坐标，
 * tile.player_ids / npc_ids 的归属变化要靠 /v1/tiles 才拿得到。
 */
import { ws } from '@/lib/ws';
import { useGameStore } from '@/store/game';

/** WorldMap 监听的事件名 */
export const PLAYER_MOVED_EVENT = 'aicity:player_moved';

/** NPCDialog 组件监听的事件名（Sprint 12 引入） */
export const NPC_DIALOGUE_EVENT = 'aicity:npc_dialogue';
export const NPC_MOVED_EVENT = 'aicity:npc_moved';

/**
 * NPCDialog 组件监听的事件名（2.0 阶段 2 引入）—— NPC 流式 sentence-by-sentence
 * + 句末 emotion chip。detail 用完整信封，消费方按 `e.detail.payload.{npc_id,
 * sentence_idx, text, emotion, session_id}` 取值。
 *
 * 信封 type 恒为 "npc_say_stream"（ws-gateway 包络）；payload 内层 type 区分
 * "npc_say_stream"（单句节拍）和 "npc_say_stream_done"（流结束）。
 */
export const NPC_SAY_STREAM_EVENT = 'aicity:npc_say_stream';

/** 与 apps/ws-gateway/internal/protocol/message.go::Envelope 一致 */
interface WsEnvelope<T> {
  type: string;
  trace_id: string;
  ts_ms: number;
  payload: T;
}

/**
 * 与 apps/world-engine/src/world_grid.rs::PlayerPosition 的 serde 输出一致。
 * 字段名是 player_id，不是 gRPC proto 里的 entity_id。
 */
export interface PlayerMovedPayload {
  player_id: string;
  tile_id: string;
  x: number;
  y: number;
  ts_ms: number;
}

/**
 * 与 apps/ws-gateway/internal/protocol/message.go::NpcDialogue + DialogOption 一致。
 *
 * 区分两种语义（消费方按 reply_to_choice_id === null 判断）：
 *   - active say：player_id=""、tile_id=""、reply_to_choice_id=null
 *     （NPC 主动说，附近所有玩家都收到，options 通常为 []）
 *   - 专属 message（welcome / reply：player_id 非空）：ws-gateway SendToPlayer 只投递到目标玩家连接；前端归属过滤是冗余保险。
 *   - reply    ：player_id/tile_id 非空、reply_to_choice_id="<choice_id>"
 *     （NPC 回复某个玩家的选项，options 至少 1 条）
 */
export interface NpcDialogOption {
  id: string;
  text: string;
}

export interface NpcMovedPayload {
  npc_id: string;
  tile_id: string;
  x: number;
  y: number;
  ts_ms: number;
}
export interface NpcDialoguePayload {
  npc_id: string;
  player_id: string;
  tile_id: string;
  say: string;
  options: NpcDialogOption[];
  reply_to_choice_id: string | null;
}

/**
 * 与 apps/agent-os/src/agent_os/stream/publisher.py::publish_beat 输出对齐。
 *
 * 字段名 (snake_case) 必须与 agent-os Publisher.publish_beat 一致；
 * web 端按 JSON key 直接取值。
 *
 * emotion 必须是 8 类之一 (EmotionValidator 已在 agent-os 端降级)，但前端
 * 做白名单兜底防 validator 漏网。
 */
export interface NpcSayStreamBeat {
  type: 'npc_say_stream';
  npc_id: string;
  session_id: string;
  sentence_idx: number;
  text: string;
  emotion: string;
  ts_ms: number;
  trace_id: string;
}

/**
 * 与 apps/agent-os/src/agent_os/stream/publisher.py::publish_done 输出对齐。
 * sentence_count 是本会话已发出的句子数（npc_say_stream 包数）；complete 是
 * 是否完整收尾（false = EndMarker 超时降级，详见 spec §3 风险 1）。
 */
export interface NpcSayStreamDone {
  type: 'npc_say_stream_done';
  npc_id: string;
  session_id: string;
  sentence_count: number;
  complete: boolean;
  ts_ms: number;
  trace_id: string;
}

/**
 * 8 类 emotion 白名单（apps/agent-os/.../emotion_validator.py::ALLOWED_EMOTIONS
 * 镜像）。web 端做兜底渲染 —— 不在表内的 emotion 一律回退 neutral emoji。
 */
export const NPC_EMOTIONS = [
  'happy',
  'sad',
  'angry',
  'surprised',
  'thinking',
  'embarrassed',
  'curious',
  'neutral',
] as const;
export type NpcEmotion = (typeof NPC_EMOTIONS)[number];

export const NPC_EMOTION_EMOJI: Record<NpcEmotion, string> = {
  happy: '😊',
  sad: '😢',
  angry: '😠',
  surprised: '😲',
  thinking: '🤔',
  embarrassed: '😳',
  curious: '🤨',
  neutral: '😐',
};

function isPlayerMoved(msg: unknown): msg is WsEnvelope<PlayerMovedPayload> {
  if (typeof msg !== 'object' || msg === null) return false;
  const m = msg as Record<string, unknown>;
  if (m.type !== 'player_moved') return false;
  const p = m.payload as Record<string, unknown> | undefined;
  return (
    typeof p === 'object' &&
    p !== null &&
    typeof p.player_id === 'string' &&
    typeof p.x === 'number' &&
    typeof p.y === 'number'
  );
}

function isNpcMoved(msg: unknown): msg is WsEnvelope<NpcMovedPayload> {
  if (typeof msg !== 'object' || msg === null) return false;
  const m = msg as Record<string, unknown>;
  if (m.type !== 'npc_moved') return false;
  const p = m.payload as Record<string, unknown> | undefined;
  return (
    typeof p === 'object' &&
    p !== null &&
    typeof p.npc_id === 'string' &&
    typeof p.x === 'number' &&
    typeof p.y === 'number'
  );
}
function isNpcDialogue(msg: unknown): msg is WsEnvelope<NpcDialoguePayload> {
  if (typeof msg !== 'object' || msg === null) return false;
  const m = msg as Record<string, unknown>;
  if (m.type !== 'npc_dialogue') return false;
  const p = m.payload as Record<string, unknown> | undefined;
  // 只校最核心两个字段；options / reply_to_choice_id 留给消费方运行时处理
  return (
    typeof p === 'object' &&
    p !== null &&
    typeof p.npc_id === 'string' &&
    typeof p.say === 'string'
  );
}

/**
 * 校验 npc_say_stream 节拍包。信包外层 (msg.type) 是 ws-gateway 包络的
 * "npc_say_stream"；内层 payload.type 也是 "npc_say_stream"。两者相等
 * 是双保险 —— 防 ws-gateway 透传错包 / 防 payload 丢字段后 type guard 漏过。
 */
function isNpcSayStreamBeat(msg: unknown): msg is WsEnvelope<NpcSayStreamBeat> {
  if (typeof msg !== 'object' || msg === null) return false;
  const m = msg as Record<string, unknown>;
  if (m.type !== 'npc_say_stream') return false;
  const p = m.payload as Record<string, unknown> | undefined;
  return (
    typeof p === 'object' &&
    p !== null &&
    p.type === 'npc_say_stream' &&
    typeof p.npc_id === 'string' &&
    typeof p.session_id === 'string' &&
    typeof p.sentence_idx === 'number' &&
    typeof p.text === 'string' &&
    typeof p.emotion === 'string'
  );
}

/**
 * 校验 npc_say_stream_done 结束标记 —— 内层 payload.type === "npc_say_stream_done"。
 * 外层 type 仍然是 ws-gateway 信封 type "npc_say_stream"（同一频道复用同一包络）。
 */
function isNpcSayStreamDone(msg: unknown): msg is WsEnvelope<NpcSayStreamDone> {
  if (typeof msg !== 'object' || msg === null) return false;
  const m = msg as Record<string, unknown>;
  if (m.type !== 'npc_say_stream') return false;
  const p = m.payload as Record<string, unknown> | undefined;
  return (
    typeof p === 'object' &&
    p !== null &&
    p.type === 'npc_say_stream_done' &&
    typeof p.npc_id === 'string' &&
    typeof p.session_id === 'string' &&
    typeof p.sentence_count === 'number' &&
    typeof p.complete === 'boolean'
  );
}

/**
 * 起 WS 桥。返回清理函数（取消订阅），可直接用于 useEffect。
 *
 * 没有 token 时不连（未登录），仍返回 no-op 清理函数让调用方无需分支。
 */
export function startWsBridge(): () => void {
  if (typeof window === 'undefined') return () => {};

  const token = window.localStorage.getItem('aicity_token');
  if (!token) {
    console.warn('[ws-bridge] no token in localStorage, skipping connect');
    return () => {};
  }

  const off = ws.onMessage((msg) => {
    if (isPlayerMoved(msg)) {
      const p = msg.payload;
      // 自己的 echo：用服务端权威坐标校准乐观更新
      if (p.player_id === useGameStore.getState().playerId) {
        useGameStore.getState().setPosition({ x: p.x, y: p.y });
      }
      // 无论谁移动都通知 WorldMap 重拉 tiles（tile.player_ids 归属可能变了）
      window.dispatchEvent(
        new CustomEvent<PlayerMovedPayload>(PLAYER_MOVED_EVENT, { detail: p })
      );
      return;
    }

    if (isNpcDialogue(msg)) {
      // Sprint 12 min slice: 只 dispatch CustomEvent 让 NPCDialog 组件订阅；
      // detail 用完整信封而非只 payload，方便消费方读 trace_id 做关联分析。
      window.dispatchEvent(
        new CustomEvent<WsEnvelope<NpcDialoguePayload>>(NPC_DIALOGUE_EVENT, { detail: msg })
      );
      return;
    }

    if (isNpcMoved(msg)) {
      // Sprint 13：NPC 移动事件广播给所有连接；WorldMap 按 npc_id 覆盖圆点坐标。
      window.dispatchEvent(
        new CustomEvent<NpcMovedPayload>(NPC_MOVED_EVENT, { detail: msg.payload })
      );
      return;
    }

    if (isNpcSayStreamBeat(msg)) {
      // 2.0 阶段 2：NPC 逐句流式推送。NPCDialog 组件订阅 → 累加 beats →
      // 切头像旁 emotion emoji。同 session 同 sentence_idx 视为重发（断连重连
      // 补帧），按 (session_id, sentence_idx) 复合键去重。
      window.dispatchEvent(
        new CustomEvent<WsEnvelope<NpcSayStreamBeat>>(
          NPC_SAY_STREAM_EVENT,
          { detail: msg }
        )
      );
      return;
    }

    if (isNpcSayStreamDone(msg)) {
      // 流结束标记（spec §2.2）—— 复用 NPC_SAY_STREAM_EVENT 频道，detail 内层
      // type 区分 beat / done。消费方按 detail.payload.type 二次分支。
      window.dispatchEvent(
        new CustomEvent<WsEnvelope<NpcSayStreamDone>>(
          NPC_SAY_STREAM_EVENT,
          { detail: msg }
        )
      );
      return;
    }
    // 未知 type：静默忽略。ws.ts 已经有 JSON.parse 兜底，这里再叠一层 type 过滤。
  });

  ws.connect(token);

  return off;
}