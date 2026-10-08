'use client';

import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';

interface ApiError {
  code: string;
  msg: string;
}

interface OceanJson {
  O: number;
  C: number;
  E: number;
  A: number;
  N: number;
}

interface ProductItem {
  name: string;
  price_gold: number;
  stock?: number | null;
}

interface NpcTemplate {
  id: number;
  name: string;
  status: string;
  price_gold: number;
  avatar_url?: string | null;
  ocean_json: OceanJson;
  bt_skeleton?: string | null;
  product_catalog?: ProductItem[] | null;
  created_at: string;
}

const OCEAN_KEYS = ['O', 'C', 'E', 'A', 'N'] as const;

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

function radarPoint(value: number, index: number, radius = 70) {
  const angle = -Math.PI / 2 + (index * 2 * Math.PI) / OCEAN_KEYS.length;
  return {
    x: 80 + Math.cos(angle) * radius * value,
    y: 80 + Math.sin(angle) * radius * value,
  };
}

function radarPath(values: OceanJson, radius = 70) {
  return OCEAN_KEYS.map((key, index) => radarPoint(values[key] ?? 0, index, radius))
    .map(({ x, y }) => `${x.toFixed(2)},${y.toFixed(2)}`)
    .join(' ');
}

export function NpcTemplateDetail({ templateId }: { templateId: number }) {
  const [purchaseState, setPurchaseState] = useState<'idle' | 'loading' | 'success'>('idle');
  const [purchaseError, setPurchaseError] = useState<ApiError | null>(null);

  const { data, isLoading, error } = useQuery<NpcTemplate>({
    queryKey: ['marketplace', 'npc-template', templateId],
    queryFn: async () => {
      const response = await fetch(`/api/marketplace/npc-templates/${templateId}`, {
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
          template_kind: 'npc',
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
        <div className="flex flex-col md:flex-row gap-6">
          {data.avatar_url && (
            // eslint-disable-next-line @next/next/no-img-element
            <img
              src={data.avatar_url}
              alt={data.name}
              className="w-32 h-32 rounded-lg object-cover"
            />
          )}
          <div className="flex-1">
            <h1 className="text-2xl font-bold">{data.name}</h1>
            <div className="text-sm text-gray-600 mb-3">{data.status}</div>
            <div className="text-2xl font-bold text-blue-600">
              {data.price_gold.toLocaleString()} Gold
            </div>
            <button
              type="button"
              onClick={purchase}
              disabled={purchaseState === 'loading'}
              className="mt-4 px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50"
            >
              {purchaseState === 'loading' ? '购买中…' : '购买'}
            </button>
            {purchaseState === 'success' && (
              <p className="mt-2 text-green-600">购买成功</p>
            )}
            {purchaseError && (
              <p className="mt-2 text-red-600">
                错误 [{purchaseError.code}]: {purchaseError.msg}
              </p>
            )}
          </div>
        </div>
      </div>

      <div className="bg-white border border-gray-200 rounded p-6">
        <h2 className="text-lg font-semibold mb-4">OCEAN</h2>
        <svg
          data-testid="ocean-radar"
          viewBox="0 0 160 160"
          className="w-64 h-64"
          role="img"
          aria-label="OCEAN radar chart"
        >
          <polygon points={radarPath({ O: 1, C: 1, E: 1, A: 1, N: 1 })} fill="none" stroke="#d1d5db" />
          <polygon
            data-testid="ocean-radar-value"
            points={radarPath(data.ocean_json)}
            fill="#3b82f633"
            stroke="#3b82f6"
          />
          {OCEAN_KEYS.map((key, index) => {
            const point = radarPoint(1, index, 82);
            return (
              <text
                key={key}
                x={point.x}
                y={point.y}
                textAnchor="middle"
                dominantBaseline="middle"
                fontSize="10"
                fill="#4b5563"
              >
                {key}
              </text>
            );
          })}
        </svg>
      </div>

      {(data.product_catalog?.length ?? 0) > 0 && (
        <div className="bg-white border border-gray-200 rounded p-6">
          <h2 className="text-lg font-semibold mb-4">商品</h2>
          <ul className="space-y-2">
            {data.product_catalog?.map((product) => (
              <li key={product.name} className="flex items-center justify-between">
                <span>{product.name}</span>
                <span className="text-gray-700">
                  {product.price_gold.toLocaleString()} Gold
                  {product.stock != null && ` · 库存 ${product.stock}`}
                </span>
              </li>
            ))}
          </ul>
        </div>
      )}

      {data.bt_skeleton && (
        <div className="bg-white border border-gray-200 rounded p-6">
          <h2 className="text-lg font-semibold mb-4">BT 骨架</h2>
          <pre className="whitespace-pre-wrap font-mono text-sm bg-gray-50 p-3 rounded">
            {data.bt_skeleton}
          </pre>
        </div>
      )}
    </div>
  );
}
