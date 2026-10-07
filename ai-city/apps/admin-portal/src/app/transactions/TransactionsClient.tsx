'use client';

import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { format } from 'date-fns';
import { usePlayerStore } from '@/lib/usePlayerStore';
import { txTypeBadge, type TxType } from '@/lib/formatTxType';

interface TxItem {
  id: number;
  type: TxType | string;
  currency: 'gold' | 'token';
  amount: number;
  balance_after: number;
  created_at: string;
  counterparty_id?: string;
  product_id?: number;
  memo?: string;
}

interface TxResponse {
  user_id: string;
  total: number;
  limit: number;
  offset: number;
  items: TxItem[];
}

const LIMIT = 50;

function describeError(e: unknown): { code: string; msg: string } {
  const raw = (e as Error)?.message ?? String(e);
  try {
    const parsed = JSON.parse(raw);
    return {
      code: parsed?.error?.code ?? 'UNKNOWN',
      msg: parsed?.error?.msg ?? raw,
    };
  } catch {
    return { code: 'UNKNOWN', msg: raw };
  }
}

function formatAmount(n: number): string {
  return n.toLocaleString('en-US');
}

export function TransactionsClient() {
  const playerId = usePlayerStore((s) => s.selectedPlayerId);
  const [offset, setOffset] = useState(0);

  const { data, isLoading, error, refetch } = useQuery<TxResponse>({
    queryKey: ['transactions', playerId, LIMIT, offset],
    queryFn: async () => {
      const pid = playerId;
      if (!pid) throw new Error('playerId missing');
      const params = new URLSearchParams({ limit: String(LIMIT), offset: String(offset) });
      const r = await fetch(`/api/economy/transactions/${pid}?${params}`);
      if (!r.ok) {
        const body = await r.json().catch(() => ({}));
        throw new Error(JSON.stringify(body));
      }
      return r.json();
    },
    enabled: !!playerId,
    retry: 1,
    staleTime: 10_000,
  });

  if (!playerId) {
    return (
      <div className="bg-yellow-50 border border-yellow-200 rounded p-4 text-sm text-yellow-800">
        请从顶部选择 player。
      </div>
    );
  }

  if (isLoading) return <p className="text-gray-500">加载中…</p>;

  if (error) {
    return (
      <p className="text-red-600" data-testid="error-banner">
        错误 [{describeError(error).code}]: {describeError(error).msg}
      </p>
    );
  }

  const total = data!.total;
  const canPrev = offset > 0;
  const canNext = offset + LIMIT < total;

  return (
    <div>
      <div className="flex items-center justify-between mb-4">
        <h1 className="text-2xl font-bold">Transactions</h1>
        <div className="flex items-center gap-3">
          <span className="text-sm text-gray-600">
            共 {total} 条 · 第 {offset / LIMIT + 1} 页
          </span>
          <button
            onClick={() => refetch()}
            className="px-3 py-1 text-sm border border-gray-300 rounded hover:bg-gray-100"
          >
            刷新
          </button>
        </div>
      </div>
      <table className="w-full bg-white border border-gray-200 text-sm" data-testid="tx-table">
        <thead className="bg-gray-50">
          <tr>
            <th className="px-3 py-2 text-left">时间</th>
            <th className="px-3 py-2 text-left">类型</th>
            <th className="px-3 py-2 text-right">货币</th>
            <th className="px-3 py-2 text-right">金额</th>
            <th className="px-3 py-2 text-right">余额</th>
            <th className="px-3 py-2 text-left">备注</th>
          </tr>
        </thead>
        <tbody>
          {data!.items.map((tx) => (
            <tr key={tx.id} className="border-t border-gray-100">
              <td className="px-3 py-2 text-gray-600">
                {format(new Date(tx.created_at), 'yyyy-MM-dd HH:mm')}
              </td>
              <td className="px-3 py-2">
                <span className={`px-2 py-0.5 rounded text-xs ${txTypeBadge(tx.type as TxType)}`}>
                  {tx.type}
                </span>
              </td>
              <td className="px-3 py-2 text-right text-gray-600">{tx.currency}</td>
              <td className={`px-3 py-2 text-right ${tx.amount < 0 ? 'text-red-600' : 'text-green-700'}`}>
                {tx.amount}
              </td>
              <td className="px-3 py-2 text-right text-gray-600">{tx.balance_after}</td>
              <td className="px-3 py-2 text-gray-600 text-xs">
                {tx.memo ?? (tx.counterparty_id && `对方 ${tx.counterparty_id.slice(0, 8)}…`) ?? (tx.product_id != null && `商品 #${tx.product_id}`) ?? ''}
              </td>
            </tr>
          ))}
          {data!.items.length === 0 && (
            <tr>
              <td colSpan={6} className="px-3 py-4 text-center text-gray-500">
                无交易记录
              </td>
            </tr>
          )}
        </tbody>
      </table>
      <div className="flex items-center justify-end gap-3 mt-4">
        <button
          onClick={() => setOffset(Math.max(0, offset - LIMIT))}
          disabled={!canPrev}
          className="px-3 py-1 text-sm border border-gray-300 rounded hover:bg-gray-100 disabled:opacity-50"
        >
          上一页
        </button>
        <button
          onClick={() => setOffset(offset + LIMIT)}
          disabled={!canNext}
          className="px-3 py-1 text-sm border border-gray-300 rounded hover:bg-gray-100 disabled:opacity-50"
        >
          下一页
        </button>
      </div>
    </div>
  );
}
