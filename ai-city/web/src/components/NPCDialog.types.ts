/**
 * NPCDialog 类型定义 —— 单独抽出避免和 ws-events.ts 形成循环 import。
 *
 * 类型大部分与 ws-events.ts 的 WsEnvelope<T> 同源，
 * 故意把 envelope 顶层字段（trace_id / ts_ms / payload）标为 optional，
 * 是为了让 CustomEvent detail 的防御性读路径（ce.detail?.payload）在
 * 极端 malformed 帧时不抛 TS error。运行时 dispatch（ws-events.ts）
 * 始终填齐全部字段，所以这是纯类型层防御，不影响 wire contract。
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
