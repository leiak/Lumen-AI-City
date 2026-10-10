'use client';

import { useEffect, useState } from 'react';
import { api, type NpcMarketTemplate, type SagaMarketTemplate } from '@/lib/api';
import { useGameStore } from '@/store/game';

interface MarketItem {
  kind: 'npc' | 'saga';
  id: number;
  name: string;
  price: number;
  description?: string | null;
}

export function CreatorMarket() {
  const sessionStatus = useGameStore((s) => s.sessionStatus);
  const [items, setItems] = useState<MarketItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [purchaseSuccess, setPurchaseSuccess] = useState<string | null>(null);
  const [buyingId, setBuyingId] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    Promise.all([api.listNpcTemplates(), api.listSagaTemplates()])
      .then(([npcTemplates, sagaTemplates]) => {
        if (cancelled) return;
        const marketItems: MarketItem[] = [
          ...npcTemplates.map((item) => ({
            kind: 'npc' as const,
            id: item.id,
            name: item.name,
            price: item.price_gold,
          })),
          ...sagaTemplates.map((item) => ({
            kind: 'saga' as const,
            id: item.id,
            name: item.name,
            price: item.price_gold,
            description: item.description,
          })),
        ];
        setItems(marketItems);
        setLoading(false);
        setError(null);
      })
      .catch(() => {
        if (cancelled) return;
        setItems([]);
        setLoading(false);
        setError('市场加载失败');
      });
    return () => {
      cancelled = true;
    };
  }, []);

  async function buy(item: MarketItem) {
    setBuyingId(`${item.kind}-${item.id}`);
    setPurchaseSuccess(null);
    setError(null);
    try {
      await api.purchaseMarketplaceTemplate({
        templateKind: item.kind,
        templateId: item.id,
        idempotencyKey: `city-ui-${item.kind}-${item.id}-${Date.now().toString(36)}`,
      });
      const refreshed = await api.getWallet();
      useGameStore.getState().setWallet({
        gold: refreshed.gold_balance,
        token: refreshed.token_balance,
      });
      setPurchaseSuccess(`已购买 ${item.name}`);
    } catch (caughtError) {
      setError(caughtError instanceof Error ? caughtError.message : '购买失败');
    } finally {
      setBuyingId(null);
    }
  }

  if (loading) {
    return <div className="mt-2 text-xs text-slate-400">创作者市场加载中...</div>;
  }

  return (
    <div className="mt-2 space-y-2 border-t border-slate-700 pt-2">
      {items.length === 0 ? (
        <div className="text-xs text-slate-400">暂无上架作品</div>
      ) : (
        items.map((item) => {
          const itemKey = `${item.kind}-${item.id}`;
          return (
            <div
              key={itemKey}
              className="flex items-center justify-between gap-2 rounded bg-slate-900/70 px-2 py-1"
            >
              <div className="min-w-0">
                <div className="truncate text-xs text-slate-100">
                  {item.name}
                  <span className="ml-1 text-slate-500">
                    {item.kind === 'npc' ? 'NPC' : '剧本'}
                  </span>
                </div>
                {item.description && (
                  <div className="truncate text-[10px] text-slate-500">{item.description}</div>
                )}
              </div>
              <button
                type="button"
                disabled={sessionStatus !== 'authenticated' || buyingId === itemKey}
                onClick={() => void buy(item)}
                className="rounded bg-brand-500 px-2 py-0.5 text-[10px] font-semibold text-white hover:bg-brand-700 disabled:cursor-not-allowed disabled:opacity-50"
              >
                {buyingId === itemKey
                  ? '购买中'
                  : sessionStatus !== 'authenticated'
                    ? '登录'
                    : `${item.price} G`}
              </button>
            </div>
          );
        })
      )}
      {error && <div className="text-xs text-red-400">{error}</div>}
      {purchaseSuccess && <div className="text-xs text-green-400">{purchaseSuccess}</div>}
    </div>
  );
}
