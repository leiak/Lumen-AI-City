'use client';

import { useEffect, useState } from 'react';
import SagaGraph from '@/components/SagaGraph';
import { SagaGraph as SagaGraphType } from '@/lib/saga_loader';

interface SagaListItem {
  name: string;
  saga_id: string;
  description: string;
}

export default function SagaVizPage() {
  const [sagas, setSagas] = useState<SagaListItem[]>([]);
  const [selected, setSelected] = useState<string | null>(null);
  const [graph, setGraph] = useState<SagaGraphType | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    fetch('/api/sagas')
      .then((r) => r.json())
      .then((data) => setSagas(data))
      .catch((e) => setError(String(e)));
  }, []);

  useEffect(() => {
    if (!selected) return;
    setLoading(true);
    setError(null);
    fetch(`/api/sagas/${selected}`)
      .then((r) => {
        if (!r.ok) throw new Error(`HTTP ${r.status}`);
        return r.json();
      })
      .then((data) => setGraph(data))
      .catch((e) => setError(String(e)))
      .finally(() => setLoading(false));
  }, [selected]);

  return (
      <main className="min-h-screen p-8">
        <header className="mb-6">
          <h1 className="text-3xl font-bold">Saga 可视化</h1>
          <p className="text-gray-600">Phase B — Saga DSL 只读流程图</p>
        </header>

        <div className="mb-4">
          <label htmlFor="saga-select" className="mr-2 font-semibold">Saga 脚本：</label>
          <select
            id="saga-select"
            className="border rounded px-3 py-2 min-w-[260px]"
            value={selected ?? ''}
            onChange={(e) => setSelected(e.target.value || null)}
            data-testid="saga-dropdown"
          >
            <option value="">— 选择 saga —</option>
            {sagas.map((s) => (
              <option key={s.name} value={s.name}>
                {s.saga_id} ({s.name})
              </option>
            ))}
          </select>
        </div>

        {error && <p className="text-red-600" data-testid="saga-error">{error}</p>}
        {loading && <p className="text-gray-600" data-testid="saga-loading">加载中…</p>}
        {graph && !loading && <SagaGraph graph={graph} />}

        {graph && (
          <section className="mt-6 text-sm text-gray-700">
            <h2 className="text-lg font-semibold mb-2">Saga 元数据</h2>
            <p><strong>ID:</strong> {graph.saga_id}</p>
            <p><strong>描述:</strong> {graph.description}</p>
            <p><strong>节点数:</strong> {graph.nodes.length}（forward={graph.nodes.filter((n) => n.type === 'forward').length}, compensation={graph.nodes.filter((n) => n.type === 'compensation').length}）</p>
            <p><strong>边数:</strong> {graph.edges.length}（forward={graph.edges.filter((e) => e.kind === 'forward').length}, compensation={graph.edges.filter((e) => e.kind === 'compensation').length}）</p>
          </section>
        )}
      </main>
  );
}
