'use client';

import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';

interface ApiError {
  code: string;
  msg: string;
}

interface SagaTemplate {
  id: number;
  name: string;
  status: string;
  price_gold: number;
  description?: string | null;
  yaml_content: string;
  npc_deps: string[];
  semantic_version: string;
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

export function SagaTemplateDetail({ templateId }: { templateId: number }) {
  const [purchaseState, setPurchaseState] = useState<'idle' | 'loading' | 'success'>('idle');
  const [purchaseError, setPurchaseError] = useState<ApiError | null>(null);

  const { data, isLoading, error } = useQuery<SagaTemplate>({
    queryKey: ['marketplace', 'saga-template', templateId],
    queryFn: async () => {
      const response = await fetch(`/api/marketplace/saga-templates/${templateId}`, {
        credentials: 'same-origin',
      });
      if (!response.ok) {
        const body = await response.json().catch(() => ({}));
        throw new Error(JSON.stringify(body));
      }
      return response.json();
    },
    retry: false,
  });

  const purchase = async () => {
    setPurchaseState('loading');
    setPurchaseError(null);
    try {
      const response = await fetch('/api/marketplace/purchase', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'same-origin',
        body: JSON.stringify({
          template_kind: 'saga',
          template_id: templateId,
          idempotency_key: crypto.randomUUID(),
        }),
      });
      if (!response.ok) {
        const body = await response.json().catch(() => ({}));
        throw new Error(JSON.stringify(body));
      }
      setPurchaseState('success');
    } catch (requestError) {
      setPurchaseState('idle');
      setPurchaseError(describeError(requestError));
    }
  };

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

  if (!data) return null;

  return (
    <div className="space-y-6">
      <div className="bg-white border border-gray-200 rounded p-6">
        <h1 className="text-2xl font-bold">{data.name}</h1>
        <div className="text-sm text-gray-600 mb-3">
          {data.status} · v{data.semantic_version}
        </div>
        {data.description && <p className="text-gray-700 mb-3">{data.description}</p>}
        <div className="text-2xl font-bold text-blue-600 mb-4">
          {data.price_gold.toLocaleString()} Gold
        </div>
        <button
          type="button"
          onClick={purchase}
          disabled={purchaseState === 'loading'}
          className="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50"
        >
          {purchaseState === 'loading' ? '购买中…' : '购买'}
        </button>
        {purchaseState === 'success' && <p className="mt-2 text-green-600">购买成功</p>}
        {purchaseError && (
          <p className="mt-2 text-red-600">
            错误 [{purchaseError.code}]: {purchaseError.msg}
          </p>
        )}
      </div>

      <div className="bg-white border border-gray-200 rounded p-6">
        <h2 className="text-lg font-semibold mb-4">YAML 预览</h2>
        <pre
          data-testid="saga-yaml-preview"
          className="whitespace-pre-wrap font-mono text-sm bg-gray-50 p-3 rounded"
        >
          {data.yaml_content}
        </pre>
      </div>

      <div className="bg-white border border-gray-200 rounded p-6">
        <h2 className="text-lg font-semibold mb-4">依赖 NPC</h2>
        {data.npc_deps.length === 0 ? (
          <p className="text-gray-600">无</p>
        ) : (
          <ul className="list-disc pl-5 space-y-1">
            {data.npc_deps.map((dep) => (
              <li key={dep}>{dep}</li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}
