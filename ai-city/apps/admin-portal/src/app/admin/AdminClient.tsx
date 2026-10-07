'use client';

import { useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';

function describeError(e: unknown): { code: string; msg: string } {
  const raw = (e as Error)?.message ?? String(e);
  try {
    const parsed = JSON.parse(raw);
    return {
      code: parsed?.error?.code ?? 'UNKNOWN',
      msg: parsed?.error?.msg ?? raw,
    };
  } catch {
    return { code: 'UNKNOWN', msg: raw };
  }
}

export function AdminClient() {
  const queryClient = useQueryClient();
  const [emitReason, setEmitReason] = useState('');
  const [sinkUserId, setSinkUserId] = useState('');
  const [sinkAmount, setSinkAmount] = useState('');
  const [sinkReason, setSinkReason] = useState('');

  const invalidateAll = () => {
    queryClient.removeQueries({ queryKey: ['wallet'] });
    queryClient.removeQueries({ queryKey: ['transactions'] });
  };

  const emitMut = useMutation({
    mutationFn: async (body: { reason: string }) => {
      const r = await fetch('/api/economy/admin/emit', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      });
      if (!r.ok) {
        const data = await r.json().catch(() => ({}));
        throw new Error(JSON.stringify(data));
      }
      return r.json();
    },
    onSuccess: invalidateAll,
  });

  const sinkMut = useMutation({
    mutationFn: async (body: { user_id: string; amount: number; reason?: string }) => {
      const r = await fetch('/api/economy/admin/sink', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      });
      if (!r.ok) {
        const data = await r.json().catch(() => ({}));
        throw new Error(JSON.stringify(data));
      }
      return r.json();
    },
    onSuccess: invalidateAll,
  });

  return (
    <div>
      <h1 className="text-2xl font-bold mb-6">Admin Tools</h1>

      {/* EMIT FORM */}
      <section className="mb-8 bg-white p-6 rounded-lg border border-gray-200 max-w-xl">
        <h2 className="text-lg font-semibold mb-3">触发 Emit（中央银行发钞）</h2>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            emitMut.mutate({ reason: emitReason });
          }}
        >
          <label className="block text-sm font-medium mb-1" htmlFor="emit-reason">
            Reason
          </label>
          <input
            id="emit-reason"
            type="text"
            value={emitReason}
            onChange={(e) => setEmitReason(e.target.value)}
            placeholder="例如 daily_emit"
            className="w-full px-3 py-2 border border-gray-300 rounded mb-3"
          />
          <button
            type="submit"
            disabled={emitMut.isPending}
            className="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50"
          >
            触发 emit
          </button>
          {emitMut.isSuccess && (
            <p className="mt-3 text-green-700 text-sm" data-testid="emit-success">
              Emit 成功 ✓（queries invalidated）
            </p>
          )}
          {emitMut.isError && (
            <p className="mt-3 text-red-600 text-sm" data-testid="emit-error">
              错误 [{describeError(emitMut.error).code}]: {describeError(emitMut.error).msg}
            </p>
          )}
        </form>
      </section>

      {/* SINK FORM */}
      <section className="bg-white p-6 rounded-lg border border-gray-200 max-w-xl">
        <h2 className="text-lg font-semibold mb-3">触发 Sink（中央银行销毁钱币）</h2>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            const amount = Number(sinkAmount);
            if (!sinkUserId || !Number.isFinite(amount) || amount <= 0) return;
            sinkMut.mutate({ user_id: sinkUserId, amount, reason: sinkReason || undefined });
          }}
        >
          <label className="block text-sm font-medium mb-1" htmlFor="sink-user">
            User ID
          </label>
          <input
            id="sink-user"
            type="text"
            value={sinkUserId}
            onChange={(e) => setSinkUserId(e.target.value)}
            placeholder="player uuid"
            className="w-full px-3 py-2 border border-gray-300 rounded mb-3"
          />
          <label className="block text-sm font-medium mb-1" htmlFor="sink-amount">
            Amount
          </label>
          <input
            id="sink-amount"
            type="number"
            value={sinkAmount}
            onChange={(e) => setSinkAmount(e.target.value)}
            placeholder="100"
            min={1}
            className="w-full px-3 py-2 border border-gray-300 rounded mb-3"
          />
          <label className="block text-sm font-medium mb-1" htmlFor="sink-reason">
            Reason (可选)
          </label>
          <input
            id="sink-reason"
            type="text"
            value={sinkReason}
            onChange={(e) => setSinkReason(e.target.value)}
            className="w-full px-3 py-2 border border-gray-300 rounded mb-3"
          />
          <button
            type="submit"
            disabled={sinkMut.isPending}
            className="px-4 py-2 bg-red-600 text-white rounded hover:bg-red-700 disabled:opacity-50"
          >
            触发 sink
          </button>
          {sinkMut.isSuccess && (
            <p className="mt-3 text-green-700 text-sm" data-testid="sink-success">
              Sink 成功 ✓（queries invalidated）
            </p>
          )}
          {sinkMut.isError && (
            <p className="mt-3 text-red-600 text-sm" data-testid="sink-error">
              错误 [{describeError(sinkMut.error).code}]: {describeError(sinkMut.error).msg}
            </p>
          )}
        </form>
      </section>
    </div>
  );
}
