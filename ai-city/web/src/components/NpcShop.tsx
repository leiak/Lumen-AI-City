'use client';

import { useEffect, useState } from 'react';
import { api, type Product } from '@/lib/api';
import { useGameStore } from '@/store/game';

interface ShopState {
  products: Product[];
  loading: boolean;
  error: string | null;
}

export function NpcShop({ npcId }: { npcId: string }) {
  const sessionStatus = useGameStore((s) => s.sessionStatus);
  const wallet = useGameStore((s) => s.wallet);
  const [state, setState] = useState<ShopState>({
    products: [],
    loading: true,
    error: null,
  });
  const [buyingId, setBuyingId] = useState<number | null>(null);
  const [purchaseSuccess, setPurchaseSuccess] = useState<string | null>(null);

  function totalGoldCost(product: Product) {
    return product.price_gold + Math.floor(product.price_gold * 0.05);
  }

  useEffect(() => {
    let cancelled = false;
    setState({ products: [], loading: true, error: null });
    api
      .listProducts(npcId)
      .then((products) => {
        if (cancelled) return;
        setState({ products, loading: false, error: null });
      })
      .catch((error) => {
        if (cancelled) return;
        setState({
          products: [],
          loading: false,
          error: error instanceof Error ? error.message : '商品加载失败',
        });
      });
    return () => {
      cancelled = true;
    };
  }, [npcId]);

  async function buy(product: Product) {
    const playerId = useGameStore.getState().playerId;
    if (!playerId) return;
    setBuyingId(product.id);
    try {
      const idempotencyKey = `city-ui-${product.id}-${Date.now().toString(36)}`;
      const purchase = await api.purchaseProduct({
        productId: product.id,
        currency: 'gold',
        idempotencyKey,
        traceId: `city-ui-${playerId}`,
      });
      const refreshed = await api.getWallet();
      useGameStore.getState().setWallet({
        gold: refreshed.gold_balance,
        token: refreshed.token_balance,
      });
      setState((prev) => ({
        ...prev,
        products: prev.products.map((item) =>
          item.id === product.id && item.stock != null
            ? { ...item, stock: Math.max(0, item.stock - 1) }
            : item,
        ),
        error: null,
      }));
      setPurchaseSuccess(
        `已购买 ${product.name} · 扣款 ${purchase.amount_paid + purchase.sink_amount} G`,
      );
      useGameStore.getState().bumpInventoryVersion();
    } catch (error) {
      setState((prev) => ({
        ...prev,
        error: error instanceof Error ? error.message : '购买失败',
      }));
      setPurchaseSuccess(null);
    } finally {
      setBuyingId(null);
    }
  }

  if (state.loading) {
    return <div className="mt-3 text-xs text-slate-400">商品加载中...</div>;
  }

  if (state.products.length === 0) {
    return null;
  }

  return (
    <div className="mt-3 rounded border border-slate-700 bg-slate-800/60 p-3">
      <div className="mb-2 text-sm font-semibold text-amber-300">商店</div>
      <div className="space-y-2">
        {state.products.map((product) => (
          <div
            key={product.id}
            className="flex items-center justify-between gap-2 rounded bg-slate-900/70 px-3 py-2"
          >
            <div className="min-w-0">
              <div className="truncate text-sm text-slate-100">{product.name}</div>
              <div className="text-xs text-slate-400">
                {product.price_gold} Gold · 实扣 {totalGoldCost(product)} G
                {product.price_token != null ? ` · ${product.price_token} Token` : ''}
                {product.stock != null ? ` · 库存 ${product.stock}` : ''}
              </div>
            </div>
            <button
              type="button"
              disabled={
                sessionStatus !== 'authenticated' ||
                buyingId !== null ||
                (product.stock != null && product.stock <= 0) ||
                (wallet != null && wallet.gold < totalGoldCost(product))
              }
              onClick={() => void buy(product)}
              className="rounded bg-brand-500 px-3 py-1 text-xs font-semibold text-white hover:bg-brand-700 disabled:cursor-not-allowed disabled:opacity-50"
            >
              {buyingId === product.id
                ? '购买中'
                : sessionStatus !== 'authenticated'
                  ? '登录购买'
                  : product.stock != null && product.stock <= 0
                    ? '缺货'
                    : wallet != null && wallet.gold < totalGoldCost(product)
                      ? '余额不足'
                      : '购买'}
            </button>
          </div>
        ))}
      </div>
      {state.error && (
        <div className="mt-2 text-xs text-red-400">{state.error}</div>
      )}
      {purchaseSuccess && (
        <div className="mt-2 text-xs text-green-400">{purchaseSuccess}</div>
      )}
      {sessionStatus !== 'authenticated' && (
        <div className="mt-2 text-xs text-slate-400">登录后可购买。</div>
      )}
    </div>
  );
}
