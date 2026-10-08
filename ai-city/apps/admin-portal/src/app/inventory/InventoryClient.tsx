'use client';

import { useQuery } from '@tanstack/react-query';

interface ApiError {
  code: string;
  msg: string;
}

interface Session {
  username: string;
  role: 'player' | 'creator' | 'admin';
}

interface InventoryItem {
  purchase_id: number;
  template_kind: 'npc' | 'saga';
  template_id: number;
  price_paid_gold: number;
  created_at: string;
}

function describeError(error: unknown): ApiError {
  const raw = (error as Error)?.message ?? String(error);
  try {
    const parsed = JSON.parse(raw);
    const apiError = (parsed?.detail ?? parsed?.error) as ApiError | undefined;
    return {
      code: apiError?.code ?? 'UNKNOWN',
      msg: apiError?.msg ?? raw,
    };
  } catch {
    return { code: 'UNKNOWN', msg: raw };
  }
}

export function InventoryClient() {
  const { data: session, isLoading: sessionLoading, error: sessionError } = useQuery({
    queryKey: ['auth-me'],
    queryFn: async () => {
      const response = await fetch('/api/auth/me', { credentials: 'same-origin' });
      if (!response.ok) {
        const body = await response.json().catch(() => ({}));
        throw new Error(JSON.stringify(body));
      }
      return response.json() as Promise<Session>;
    },
    retry: false,
    staleTime: 60_000,
  });

  const {
    data,
    isLoading: inventoryLoading,
    error: inventoryError,
    refetch,
  } = useQuery<InventoryItem[]>({
    queryKey: ['marketplace', 'inventory', session?.username],
    queryFn: async () => {
      const response = await fetch(
        `/api/marketplace/inventory/${encodeURIComponent(session!.username)}?limit=50&offset=0`,
        { credentials: 'same-origin' },
      );
      if (!response.ok) {
        const body = await response.json().catch(() => ({}));
        throw new Error(JSON.stringify(body));
      }
      return response.json();
    },
    enabled: !!session,
    retry: false,
    staleTime: 10_000,
  });

  if (sessionLoading || (session && inventoryLoading)) {
    return <p className="text-gray-500">加载中…</p>;
  }

  if (sessionError || inventoryError) {
    const { code, msg } = describeError(sessionError ?? inventoryError);
    return (
      <p className="text-red-600">
        错误 [{code}]: {msg}
      </p>
    );
  }

  if (!session) {
    return <p className="text-gray-600">请先登录。</p>;
  }

  const items = data ?? [];

  return (
    <div>
      <div className="flex items-center justify-between mb-4">
        <h1 className="text-2xl font-bold">Inventory</h1>
        <button
          type="button"
          onClick={() => refetch()}
          className="px-3 py-1 text-sm border border-gray-300 rounded hover:bg-gray-100"
        >
          刷新
        </button>
      </div>

      {items.length === 0 ? (
        <div className="bg-white border border-gray-200 rounded p-6 text-gray-600">
          暂无已购模板
        </div>
      ) : (
        <div className="overflow-x-auto bg-white border border-gray-200 rounded">
          <table className="min-w-full divide-y divide-gray-200">
            <thead className="bg-gray-50">
              <tr>
                <th className="px-4 py-2 text-left text-sm font-semibold text-gray-700">Purchase</th>
                <th className="px-4 py-2 text-left text-sm font-semibold text-gray-700">Kind</th>
                <th className="px-4 py-2 text-left text-sm font-semibold text-gray-700">Template</th>
                <th className="px-4 py-2 text-right text-sm font-semibold text-gray-700">Price</th>
                <th className="px-4 py-2 text-left text-sm font-semibold text-gray-700">Created</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {items.map((item) => (
                <tr key={item.purchase_id} data-testid={`inventory-row-${item.purchase_id}`}>
                  <td className="px-4 py-2 text-sm text-gray-700">{item.purchase_id}</td>
                  <td className="px-4 py-2 text-sm text-gray-700">{item.template_kind}</td>
                  <td className="px-4 py-2 text-sm font-medium text-gray-900">{item.template_id}</td>
                  <td className="px-4 py-2 text-sm text-right text-gray-700">
                    {item.price_paid_gold.toLocaleString()}
                  </td>
                  <td className="px-4 py-2 text-sm text-gray-600">
                    {new Date(item.created_at).toLocaleString()}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
