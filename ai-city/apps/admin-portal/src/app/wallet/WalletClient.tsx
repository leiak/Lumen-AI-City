'use client';

import { useQuery } from '@tanstack/react-query';
import { usePlayerStore } from '@/lib/usePlayerStore';

interface Wallet {
  user_id: string;
  gold_balance: number;
  token_balance: number;
}

interface ApiError {
  code: string;
  msg: string;
}

function formatNumber(n: number): string {
  return n.toLocaleString('en-US');
}

/** Unwrap a `{ error: { code, msg } }` envelope that was JSON.stringify'd into
 *  the thrown Error's message. Falls back to UNKNOWN + the raw string. */
function describeError(e: unknown): { code: string; msg: string } {
  const raw = (e as Error)?.message ?? String(e);
  try {
    const parsed = JSON.parse(raw);
    const errObj = parsed?.error as ApiError | undefined;
    return {
      code: errObj?.code ?? 'UNKNOWN',
      msg: errObj?.msg ?? raw,
    };
  } catch {
    return { code: 'UNKNOWN', msg: raw };
  }
}

function BalanceCard({ label, value }: { label: string; value: number }) {
  return (
    <div className="bg-white p-6 rounded-lg border border-gray-200">
      <div className="text-sm text-gray-600 mb-1">{label}</div>
      <div data-testid={`balance-${label.toLowerCase().replace(' ', '-')}`} className="text-3xl font-bold">
        {formatNumber(value)}
      </div>
    </div>
  );
}

export function WalletClient() {
  const playerId = usePlayerStore((s) => s.selectedPlayerId);

  const { data, isLoading, error, refetch } = useQuery<Wallet>({
    queryKey: ['wallet', playerId],
    queryFn: async () => {
      const pid = playerId;
      if (!pid) throw new Error('playerId missing');
      const r = await fetch(`/api/economy/wallet/${encodeURIComponent(pid)}`);
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
    const { code, msg } = describeError(error);
    return (
      <p className="text-red-600">
        错误 [{code}]: {msg}
      </p>
    );
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-4">
        <h1 className="text-2xl font-bold">Wallet</h1>
        <button
          onClick={() => refetch()}
          className="px-3 py-1 text-sm border border-gray-300 rounded hover:bg-gray-100"
        >
          刷新
        </button>
      </div>
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4 max-w-2xl">
        <BalanceCard label="Gold Balance" value={data!.gold_balance} />
        <BalanceCard label="Token Balance" value={data!.token_balance} />
      </div>
    </div>
  );
}