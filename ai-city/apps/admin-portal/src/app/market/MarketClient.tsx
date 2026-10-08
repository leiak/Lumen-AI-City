'use client';

import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';

interface ApiError {
  code: string;
  msg: string;
}

interface TemplateSummary {
  id: number;
  name: string;
  status: string;
  price_gold: number;
}

type TemplateKind = 'npc' | 'saga';

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

export function MarketClient() {
  const [kind, setKind] = useState<TemplateKind>('npc');
  const [search, setSearch] = useState('');

  const { data, isLoading, error, refetch } = useQuery<TemplateSummary[]>({
    queryKey: ['marketplace', 'browse', kind],
    queryFn: async () => {
      const response = await fetch(
        `/api/marketplace/${kind}-templates?limit=50&offset=0&status=live`,
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

  const normalizedSearch = search.trim().toLowerCase();
  const templates = (data ?? []).filter((template) =>
    template.name.toLowerCase().includes(normalizedSearch),
  );

  return (
    <div>
      <div className="flex flex-col md:flex-row md:items-center md:justify-between gap-4 mb-4">
        <h1 className="text-2xl font-bold">Market</h1>
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={() => setKind('npc')}
            className={`px-3 py-1 text-sm rounded ${
              kind === 'npc' ? 'bg-blue-600 text-white' : 'border border-gray-300 hover:bg-gray-100'
            }`}
          >
            NPC
          </button>
          <button
            type="button"
            onClick={() => setKind('saga')}
            className={`px-3 py-1 text-sm rounded ${
              kind === 'saga' ? 'bg-blue-600 text-white' : 'border border-gray-300 hover:bg-gray-100'
            }`}
          >
            Saga
          </button>
          <button
            type="button"
            onClick={() => refetch()}
            className="px-3 py-1 text-sm border border-gray-300 rounded hover:bg-gray-100"
          >
            刷新
          </button>
        </div>
      </div>

      <div className="mb-4">
        <label htmlFor="market-search" className="sr-only">
          搜索
        </label>
        <input
          id="market-search"
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          placeholder="搜索模板名称"
          className="w-full md:w-80 border border-gray-300 rounded px-3 py-2"
        />
      </div>

      {isLoading ? (
        <p className="text-gray-500">加载中…</p>
      ) : error ? (
        (() => {
          const { code, msg } = describeError(error);
          return (
            <p className="text-red-600">
              错误 [{code}]: {msg}
            </p>
          );
        })()
      ) : templates.length === 0 ? (
        <div data-testid="market-empty" className="bg-white border border-gray-200 rounded p-6 text-gray-600">
          暂无模板
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
          {templates.map((template) => (
            <a
              key={template.id}
              href={`/market/${kind}-templates/${template.id}`}
              data-testid={`${kind}-card-${template.id}`}
              className="block bg-white border border-gray-200 rounded p-4 hover:border-blue-400"
            >
              <div className="text-lg font-semibold text-gray-900">{template.name}</div>
              <div className="text-sm text-gray-600 mb-3">{template.status}</div>
              <div className="text-xl font-bold text-blue-600">
                {template.price_gold.toLocaleString()} Gold
              </div>
            </a>
          ))}
        </div>
      )}
    </div>
  );
}
