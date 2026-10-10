'use client';

import { useEffect, useState } from 'react';
import { api, type CreatorRevenueItem } from '@/lib/api';

export function CreatorRevenue() {
  const [revenue, setRevenue] = useState<CreatorRevenueItem[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    api
      .getCreatorRevenue()
      .then((items) => {
        if (cancelled) return;
        setRevenue(items);
        setError(null);
      })
      .catch((caughtError: unknown) => {
        if (cancelled) return;
        setRevenue(null);
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
