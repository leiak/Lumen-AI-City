/**
 * NPCDialog 类型定义 —— 单独抽出避免和 ws-events.ts 形成循环 import。
 *
 * ws-events.ts 同时被 client（这里）和 server-only 路径引用；类型按值搬过来
 * 保持结构同源；运行时仍走 ws-events 的 NPC_DIALOGUE_EVENT 常量。
 */
export interface NpcDialogOption {
  id: string;
  text: string;
}

export interface NpcDialoguePayload {
  npc_id: string;
  player_id: string;
  tile_id: string;
  say: string;
  options: NpcDialogOption[];
  reply_to_choice_id: string | null;
}

export interface WsEnvelopeLike {
  type: string;
  trace_id?: string;
  ts_ms?: number;
  payload?: NpcDialoguePayload;
}
