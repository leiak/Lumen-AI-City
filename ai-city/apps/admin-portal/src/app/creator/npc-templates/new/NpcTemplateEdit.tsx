'use client';

import { useRouter } from 'next/navigation';
import { useState } from 'react';

interface ApiError {
  code: string;
  msg: string;
}

interface ProductDraft {
  name: string;
  price_gold: string;
  stock: string;
}

const OCEAN_FIELDS = [
  { key: 'O', label: '开放性 O' },
  { key: 'C', label: '尽责性 C' },
  { key: 'E', label: '外向性 E' },
  { key: 'A', label: '宜人性 A' },
  { key: 'N', label: '神经质 N' },
] as const;

type OceanKey = (typeof OCEAN_FIELDS)[number]['key'];

const DEFAULT_OCEAN: Record<OceanKey, string> = {
  O: '0.5',
  C: '0.5',
  E: '0.5',
  A: '0.5',
  N: '0.5',
};

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

export function NpcTemplateEdit() {
  const router = useRouter();
  const [name, setName] = useState('');
  const [avatarUrl, setAvatarUrl] = useState('');
  const [priceGold, setPriceGold] = useState('10');
  const [btSkeleton, setBtSkeleton] = useState('');
  const [ocean, setOcean] = useState(DEFAULT_OCEAN);
  const [products, setProducts] = useState<ProductDraft[]>([]);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<ApiError | null>(null);

  const updateOcean = (key: OceanKey, value: string) => {
    setOcean((current) => ({ ...current, [key]: value }));
  };

  const updateProduct = (index: number, patch: Partial<ProductDraft>) => {
    setProducts((current) =>
      current.map((product, productIndex) =>
        productIndex === index ? { ...product, ...patch } : product,
      ),
    );
  };

  const submit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setError(null);
    setSubmitting(true);

    const oceanJson = Object.fromEntries(
      OCEAN_FIELDS.map(({ key }) => [key, Number(ocean[key])]),
    );
    const validProducts = products.filter(
      (product) => product.name.trim() && product.price_gold !== '',
    );
    const payload = {
      name: name.trim(),
      price_gold: Number(priceGold),
      ocean_json: oceanJson,
      ...(btSkeleton.trim() ? { bt_skeleton: btSkeleton } : {}),
      ...(avatarUrl.trim() ? { avatar_url: avatarUrl.trim() } : {}),
      ...(validProducts.length
        ? {
            product_catalog: validProducts.map((product) => ({
              name: product.name.trim(),
              price_gold: Number(product.price_gold),
              ...(product.stock !== ''
                ? { stock: Number(product.stock) }
                : {}),
            })),
          }
        : {}),
    };

    try {
      const response = await fetch('/api/marketplace/npc-templates', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'same-origin',
        body: JSON.stringify(payload),
      });
      if (!response.ok) {
        const body = await response.json().catch(() => ({}));
        throw new Error(JSON.stringify(body));
      }
      router.push('/creator/npc-templates');
    } catch (requestError) {
      setError(describeError(requestError));
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <form onSubmit={submit} className="max-w-3xl space-y-6">
      <div>
        <h1 className="text-2xl font-bold">Create NPC Template</h1>
      </div>

      {error && (
        <p className="text-red-600">
          错误 [{error.code}]: {error.msg}
        </p>
      )}

      <div className="bg-white border border-gray-200 rounded p-4 space-y-4">
        <div>
          <label htmlFor="npc-name" className="block text-sm font-medium text-gray-700">
            名称
          </label>
          <input
            id="npc-name"
            value={name}
            onChange={(event) => setName(event.target.value)}
            required
            maxLength={64}
            className="mt-1 w-full border border-gray-300 rounded px-3 py-2"
          />
        </div>
        <div>
          <label htmlFor="npc-avatar-url" className="block text-sm font-medium text-gray-700">
            头像 URL
          </label>
          <input
            id="npc-avatar-url"
            value={avatarUrl}
            onChange={(event) => setAvatarUrl(event.target.value)}
            className="mt-1 w-full border border-gray-300 rounded px-3 py-2"
          />
        </div>
        <div>
          <label htmlFor="npc-price-gold" className="block text-sm font-medium text-gray-700">
            售价（Gold）
          </label>
          <input
            id="npc-price-gold"
            type="number"
            min={10}
            value={priceGold}
            onChange={(event) => setPriceGold(event.target.value)}
            required
            className="mt-1 w-40 border border-gray-300 rounded px-3 py-2"
          />
        </div>
      </div>

      <div className="bg-white border border-gray-200 rounded p-4">
        <h2 className="text-lg font-semibold mb-4">OCEAN</h2>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          {OCEAN_FIELDS.map(({ key, label }) => (
            <div key={key}>
              <label
                htmlFor={`npc-ocean-${key.toLowerCase()}`}
                className="block text-sm font-medium text-gray-700"
              >
                {label}
              </label>
              <input
                id={`npc-ocean-${key.toLowerCase()}`}
                type="range"
                min={0}
                max={1}
                step={0.01}
                value={ocean[key]}
                onChange={(event) => updateOcean(key, event.target.value)}
                className="w-full"
              />
              <div className="text-sm text-gray-600">{Number(ocean[key]).toFixed(2)}</div>
            </div>
          ))}
        </div>
      </div>

      <div className="bg-white border border-gray-200 rounded p-4">
        <label htmlFor="npc-bt-skeleton" className="block text-sm font-medium text-gray-700">
          BT 骨架
        </label>
        <textarea
          id="npc-bt-skeleton"
          value={btSkeleton}
          onChange={(event) => setBtSkeleton(event.target.value)}
          rows={10}
          className="mt-1 w-full border border-gray-300 rounded px-3 py-2 font-mono"
        />
      </div>

      <div className="bg-white border border-gray-200 rounded p-4">
        <div className="flex items-center justify-between mb-3">
          <h2 className="text-lg font-semibold">商品</h2>
          <button
            type="button"
            onClick={() =>
              setProducts((current) => [
                ...current,
                { name: '', price_gold: '', stock: '' },
              ])
            }
            className="px-3 py-1 text-sm border border-gray-300 rounded hover:bg-gray-100"
          >
            添加商品
          </button>
        </div>
        {products.map((product, index) => (
          <div key={index} className="grid grid-cols-1 md:grid-cols-4 gap-3 mb-3">
            <div>
              <label htmlFor={`product-name-${index}`} className="block text-sm text-gray-700">
                商品名称
              </label>
              <input
                id={`product-name-${index}`}
                value={product.name}
                onChange={(event) => updateProduct(index, { name: event.target.value })}
                className="mt-1 w-full border border-gray-300 rounded px-3 py-2"
              />
            </div>
            <div>
              <label htmlFor={`product-price-${index}`} className="block text-sm text-gray-700">
                商品价格
              </label>
              <input
                id={`product-price-${index}`}
                type="number"
                min={0}
                value={product.price_gold}
                onChange={(event) => updateProduct(index, { price_gold: event.target.value })}
                className="mt-1 w-full border border-gray-300 rounded px-3 py-2"
              />
            </div>
            <div>
              <label htmlFor={`product-stock-${index}`} className="block text-sm text-gray-700">
                库存
              </label>
              <input
                id={`product-stock-${index}`}
                type="number"
                min={0}
                value={product.stock}
                onChange={(event) => updateProduct(index, { stock: event.target.value })}
                className="mt-1 w-full border border-gray-300 rounded px-3 py-2"
              />
            </div>
            <div className="flex items-end">
              <button
                type="button"
                onClick={() =>
                  setProducts((current) => current.filter((_, itemIndex) => itemIndex !== index))
                }
                className="px-3 py-2 text-sm border border-gray-300 rounded hover:bg-gray-100"
              >
                移除
              </button>
            </div>
          </div>
        ))}
      </div>

      <button
        type="submit"
        disabled={submitting}
        className="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50"
      >
        {submitting ? '提交中…' : '创建模板'}
      </button>
    </form>
  );
}
