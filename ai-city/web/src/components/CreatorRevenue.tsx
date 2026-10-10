'use client';

import { useEffect, useState } from 'react';
import { useGameStore } from '@/store/game';
import {
  api,
  type CreatorRevenueItem,
  type CreatorRevenueSummary,
} from '@/lib/api';

export function CreatorRevenue() {
  const [revenue, setRevenue] = useState<CreatorRevenueItem[] | null>(null);
  const [summary, setSummary] = useState<CreatorRevenueSummary | null>(null);
  const [withdrawing, setWithdrawing] = useState(false);
  const [withdrawError, setWithdrawError] = useState<string | null>(null);
  const [withdrawSuccess, setWithdrawSuccess] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    Promise.all([
      api.getCreatorRevenue(),
      api.getCreatorRevenueSummary(),
    ])
      .then(([items, revenueSummary]) => {
        if (cancelled) return;
        setRevenue(items);
        setSummary(revenueSummary);
        setError(null);
      })
      .catch((caughtError: unknown) => {
        if (cancelled) return;
        setRevenue(null);
        setSummary(null);
        setError(
          caughtError instanceof Error && caughtError.message.includes('403')
            ? '需要 creator 或 admin 角色'
            : '收益加载失败',
        );
      });
    return () => {
      cancelled = true;
    };
  }, []);

  async function withdrawRevenue() {
    setWithdrawing(true);
    setWithdrawError(null);
    setWithdrawSuccess(null);
    try {
      const result = await api.withdrawCreatorRevenue();
      const currentWallet = useGameStore.getState().wallet;
      useGameStore.getState().setWallet({
        gold: result.balance_after,
        token: currentWallet?.token ?? 0,
      });
      const [items, revenueSummary] = await Promise.all([
        api.getCreatorRevenue(),
        api.getCreatorRevenueSummary(),
      ]);
      setRevenue(items);
      setSummary(revenueSummary);
      setWithdrawSuccess('提现成功');
    } catch (caughtError) {
      setWithdrawError(
        caughtError instanceof Error && caughtError.message.includes('R_035')
          ? '暂无可提现分成'
          : '提现失败',
      );
    } finally {
      setWithdrawing(false);
    }
  }

  const totalGold =
    revenue?.reduce((sum, item) => sum + item.amount_gold, 0) ?? 0;

  return (
    <div className="mt-2 space-y-2 border-t border-slate-700 pt-2">
      {error ? (
        <div className="text-xs text-red-400">{error}</div>
      ) : revenue == null ? (
        <div className="text-xs text-slate-400">收益加载中...</div>
      ) : (
        <>
          <div className="text-xs font-semibold text-amber-300">
            总收益: {totalGold} Gold
          </div>
          {summary && (
            <div className="text-xs text-slate-400">
              已结算: {summary.withdrawn_gold} Gold · 可提现:{' '}
              {summary.available_gold} Gold
            </div>
          )}
          <button
            type="button"
            disabled={withdrawing || !summary || summary.available_gold <= 0}
            onClick={() => void withdrawRevenue()}
            className="w-full rounded bg-brand-500 px-3 py-1 text-xs font-semibold text-white hover:bg-brand-700 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {withdrawing
              ? '提现中'
              : summary && summary.available_gold > 0
                ? `提现 ${summary.available_gold} Gold`
                : '暂无可提现分成'}
          </button>
          {withdrawError && (
            <div className="text-xs text-red-400">{withdrawError}</div>
          )}
          {withdrawSuccess && (
            <div className="text-xs text-green-400">{withdrawSuccess}</div>
          )}
          {revenue.length === 0 ? (
            <div className="text-xs text-slate-400">暂无分成记录</div>
          ) : (
            revenue.map((item) => (
              <div
                key={item.purchase_id}
                className="flex items-center justify-between gap-2 text-xs"
              >
                <span className="text-slate-300">#{item.purchase_id}</span>
                <span className="text-green-400">
                  +{item.amount_gold} Gold
                  {item.platform_cut_gold > 0 &&
                    ` · 平台 ${item.platform_cut_gold}`}
                </span>
              </div>
            ))
          )}
        </>
      )}
    </div>
  );
}
