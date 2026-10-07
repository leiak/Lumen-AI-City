'use client';

import { useQuery } from '@tanstack/react-query';
import { usePlayerStore } from '@/lib/usePlayerStore';

interface Wallet {
  user_id: string;
  gold_balance: number;
  token_balance: number;
}

function formatNumber(n: number): string {
  return n.toLocaleString('en-US');
}

function BalanceCard({ label, value, onRefresh }: { label: string; value: number; onRefresh: () => void }) {
  return (
    <div className="bg-white p-6 rounded-lg border border-gray-200">
      <div className="text-sm text-gray-600 mb-1">{label}</div>
      <div data-testid={`balance-${label.toLowerCase().replace(' ', '-')}`} className="text-3xl font-bold">
        {formatNumber(value)}
      </div>
      <button
        onClick={onRefresh}
        className="mt-3 px-3 py-3 text-sm border border-gray-300 rounded hover:bg-gray-100"
      >
        刷新
      </button>
    </div>
  );
}

export function WalletClient() {
  const playerId = usePlayerStore((s) => s.selectedPlayerId);

  const { data, isLoading, error, refetch } = useQuery<Wallet>({
    queryKey: ['wallet', playerId],
    queryFn: () =>
      fetch(`/api/economy/wallet/${encodeURIComponent(playerId!)}`).then((r) => r.json()),
    enabled: !!playerId,
  });

  if (!playerId) {
    return (
      <div className="bg-yellow-50 border border-yellow-200 rounded p-4 text-sm text-yellow-800">
        请从顶部选择 player。
      </div>
    );
  }

  if (isLoading) return <p className="text-gray-500">加载中…</p>;

  if (error || !data) {
    return <p className="text-red-600">错误: {String(error ?? 'no data')}</p>;
  }

  return (
    <div>
      <h1 className="text-2xl font-bold mb-4">Wallet</h1>
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4 max-w-2xl">
        <BalanceCard label="Gold Balance" value={data.gold_balance} onRefresh={() => refetch()} />
        <BalanceCard label="Token Balance" value={data.token_balance} onRefresh={() => refetch()} />
      </div>
    </div>
  );
}