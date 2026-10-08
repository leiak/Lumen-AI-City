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

interface NpcTemplateSummary {
  id: number;
  name: string;
  status: string;
  price_gold: number;
  created_at: string;
}

function describeError(error: unknown): ApiError {
  const raw = (error as Error)?.message ?? String(error);
  try {
    const parsed = JSON.parse(raw);
    const apiError = parsed?.error as ApiError | undefined;
    return {
      code: apiError?.code ?? 'UNKNOWN',
      msg: apiError?.msg ?? raw,
    };
  } catch {
    return { code: 'UNKNOWN', msg: raw };
  }
}

export function NpcTemplateList() {
  const { data: session } = useQuery({
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

  const { data, isLoading, error, refetch } = useQuery<NpcTemplateSummary[]>({
    queryKey: ['marketplace', 'npc-templates', 'live'],
    queryFn: async () => {
      const response = await fetch(
        '/api/marketplace/npc-templates?limit=50&offset=0&status=live',
        { credentials: 'same-origin' },
      );
      if (!response.ok) {
        const body = await response.json().catch(() => ({}));
        throw new Error(JSON.stringify(body));
      }
      return response.json();
    },
    retry: false,
    staleTime: 10_000,
  });

  const canCreate = session?.role === 'creator' || session?.role === 'admin';
  const templates = data ?? [];

  if (isLoading) {
    return <p className="text-gray-500">加载中…</p>;
  }

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
        <h1 className="text-2xl font-bold">NPC Templates</h1>
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={() => refetch()}
            className="px-3 py-1 text-sm border border-gray-300 rounded hover:bg-gray-100"
          >
            刷新
          </button>
          {canCreate && (
            <a
              href="/creator/npc-templates/new"
              className="px-3 py-1 text-sm bg-blue-600 text-white rounded hover:bg-blue-700"
            >
              创建
            </a>
          )}
        </div>
      </div>

      {templates.length === 0 ? (
        <div className="bg-white border border-gray-200 rounded p-6 text-gray-600">
          暂无 NPC 模板
        </div>
      ) : (
        <div className="overflow-x-auto bg-white border border-gray-200 rounded">
          <table className="min-w-full divide-y divide-gray-200">
            <thead className="bg-gray-50">
              <tr>
                <th className="px-4 py-2 text-left text-sm font-semibold text-gray-700">ID</th>
                <th className="px-4 py-2 text-left text-sm font-semibold text-gray-700">Name</th>
                <th className="px-4 py-2 text-left text-sm font-semibold text-gray-700">Status</th>
                <th className="px-4 py-2 text-right text-sm font-semibold text-gray-700">Price</th>
                <th className="px-4 py-2 text-left text-sm font-semibold text-gray-700">Created</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {templates.map((template) => (
                <tr key={template.id} data-testid={`npc-template-row-${template.id}`}>
                  <td className="px-4 py-2 text-sm text-gray-700">{template.id}</td>
                  <td className="px-4 py-2 text-sm font-medium text-gray-900">{template.name}</td>
                  <td className="px-4 py-2 text-sm text-gray-600">{template.status}</td>
                  <td className="px-4 py-2 text-sm text-right text-gray-700">
                    {template.price_gold.toLocaleString()}
                  </td>
                  <td className="px-4 py-2 text-sm text-gray-600">
                    {new Date(template.created_at).toLocaleString()}
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
