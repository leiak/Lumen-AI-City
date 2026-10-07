export type TxType = 'player_transfer' | 'npc_purchase' | 'central_bank_emit' | 'npc_sink';

const TX_TYPE_LABELS: Record<TxType, string> = {
  player_transfer: '玩家转账',
  npc_purchase: 'NPC 购买',
  central_bank_emit: '中央银行发钞',
  npc_sink: 'NPC sink',
};

const TX_TYPE_BADGE: Record<TxType, string> = {
  player_transfer: 'bg-blue-100 text-blue-700',
  npc_purchase: 'bg-green-100 text-green-700',
  central_bank_emit: 'bg-purple-100 text-purple-700',
  npc_sink: 'bg-red-100 text-red-700',
};

export function formatTxType(t: TxType): string {
  return TX_TYPE_LABELS[t] ?? t;
}

export function txTypeBadge(t: TxType): string {
  return TX_TYPE_BADGE[t] ?? 'bg-gray-100 text-gray-700';
}
