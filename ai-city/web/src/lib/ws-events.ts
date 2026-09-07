/**
 * WS 推送 → 应用状态的桥（Sprint 9）。
 *
 * 取代 WorldMap 原来的 3s 轮询：ws-gateway 收到 world-engine 的
 * `aicity:player:moved` 后立刻扇出，这里把信封分派成：
 *   - 自己的 move echo   → store.setPosition 校准（服务端权威坐标）
 *   - 任何玩家的 move    → dispatch `aicity:player_moved` CustomEvent，
 *                          WorldMap 收到后 debounce 重拉 /v1/tiles
 *
 * 为什么还要重拉 tiles：信封只带一个玩家的新坐标，tile.player_ids /
 * npc_ids 的归属变化要靠 /v1/tiles 才拿得到。推送在这里只当"失效通知"用。
 */
import { ws } from '@/lib/ws';
import { useGameStore } from '@/store/game';

/** WorldMap 监听的事件名 */
export const PLAYER_MOVED_EVENT = 'aicity:player_moved';

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
    if (!isPlayerMoved(msg)) return;

    const p = msg.payload;
    // 自己的 echo：用服务端权威坐标校准乐观更新
    if (p.player_id === useGameStore.getState().playerId) {
      useGameStore.getState().setPosition({ x: p.x, y: p.y });
    }

    // 无论谁移动都通知 WorldMap 重拉 tiles（tile.player_ids 归属可能变了）
    window.dispatchEvent(new CustomEvent<PlayerMovedPayload>(PLAYER_MOVED_EVENT, { detail: p }));
  });

  ws.connect(token);

  return off;
}
