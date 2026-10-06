'use client';

import { useCallback, useEffect, useMemo, useState } from 'react';
import dynamic from 'next/dynamic';
import Editor from '@monaco-editor/react';
import {
  listBtTrees,
  getBtTree,
  saveBtTree,
  simulateBtTree,
  type BTTreeSummary,
  type BTTreeFull,
  type BTSimulateResponse,
} from '@/lib/bt_api';
import { parseBtTreeToGraph, type BTGraph, type BTJSON } from '@/lib/bt_loader';
import { extractApiError } from '@/lib/extract_api_error';

// Monaco is client-only — React Flow too. Dynamic imports avoid SSR cost.
const BTGraph = dynamic(() => import('@/components/BTGraph'), { ssr: false });

/** Hard-coded NPC stub list (Phase C.3) — Phase C.4 wires the real NPC
 *  registry. Kept small but covers the seeded NPCs from the acceptance
 *  smoke so the dropdown is meaningful in dev. */
const NPC_STUB: Array<{ id: string; label: string }> = [
  { id: 'npc_wang_boss_001', label: '王老板 (城主)' },
  { id: 'npc_grace_healer_001', label: 'Grace (护士)' },
  { id: 'npc_book_keeper_001', label: '书店老板' },
  { id: 'npc_li_blacksmith_001', label: '李铁匠' },
  { id: 'npc_mei_merchant_001', label: '梅商人' },
];

interface Props {
  initialNpcId: string;
  initialTreeName?: string | null;
}

type ToastKind = 'success' | 'error' | 'warning';
type Toast = { kind: ToastKind; msg: string } | null;

export default function BtEditorClient({ initialNpcId, initialTreeName }: Props) {
  const [npcId, setNpcId] = useState(initialNpcId);
  const [treeName, setTreeName] = useState<string | null>(initialTreeName ?? null);
  const [treeFull, setTreeFull] = useState<BTTreeFull | null>(null);
  const [treeJson, setTreeJson] = useState<BTJSON | null>(null);
  const [trees, setTrees] = useState<BTTreeSummary[]>([]);
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);
  const [trace, setTrace] = useState<BTSimulateResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [simulating, setSimulating] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [toast, setToast] = useState<Toast>(null);

  // ---- Fetch tree list when NPC changes ----
  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    setTrees([]);
    setTreeFull(null);
    setTreeJson(null);
    setTrace(null);
    listBtTrees(npcId)
      .then((list) => {
        if (cancelled) return;
        setTrees(list);
        if (list.length > 0 && !list.some((t) => t.name === treeName)) {
          setTreeName(list[0].name);
        } else if (list.length === 0) {
          setTreeName(null);
        }
      })
      .catch((e) => {
        if (!cancelled) setError(String(e));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [npcId]);

  // ---- Fetch one tree when tree name changes ----
  useEffect(() => {
    if (!treeName) {
      setTreeFull(null);
      setTreeJson(null);
      return;
    }
    let cancelled = false;
    setLoading(true);
    setError(null);
    setTrace(null);
    getBtTree(npcId, treeName)
      .then((t) => {
        if (cancelled) return;
        if (!t) {
          setError(`Tree "${treeName}" not found for ${npcId}`);
          setTreeFull(null);
          setTreeJson(null);
          return;
        }
        setTreeFull(t);
        setTreeJson(t.tree_json);
      })
      .catch((e) => {
        if (!cancelled) setError(String(e));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [npcId, treeName]);

  // ---- Parse BTJSON → graph for the renderer ----
  const graph: BTGraph | null = useMemo(() => {
    if (!treeJson) return null;
    try {
      return parseBtTreeToGraph(treeJson);
    } catch (e) {
      setError(`Parse error: ${String(e)}`);
      return null;
    }
  }, [treeJson]);

  // ---- Handlers ----
  const showToast = useCallback((kind: ToastKind, msg: string) => {
    setToast({ kind, msg });
    window.setTimeout(() => setToast(null), 3000);
  }, []);

  const handleSave = useCallback(async () => {
    if (!treeName || !treeJson) return;
    setSaving(true);
    setError(null);
    try {
      const saved = await saveBtTree(npcId, treeName, treeJson);
      setTreeFull(saved);
      showToast('success', `已保存 (version ${saved.version})`);
      // refresh list
      const list = await listBtTrees(npcId);
      setTrees(list);
    } catch (e) {
      showToast('error', extractApiError(e));
    } finally {
      setSaving(false);
    }
  }, [npcId, treeName, treeJson, showToast]);

  const handleSimulate = useCallback(async () => {
    if (!treeName || !treeJson) return;
    setSimulating(true);
    setError(null);
    try {
      const result = await simulateBtTree(npcId, treeName, treeJson, 100);
      setTrace(result);
      showToast('success', `模拟完成: ${result.status}`);
    } catch (e) {
      showToast('error', extractApiError(e));
    } finally {
      setSimulating(false);
    }
  }, [npcId, treeName, treeJson, showToast]);

  const handleEditorChange = useCallback((value: string | undefined) => {
    if (value === undefined) return;
    try {
      const parsed = JSON.parse(value) as BTJSON;
      setTreeJson(parsed);
      setError(null);
    } catch (e) {
      setError(`JSON parse: ${String(e)}`);
    }
  }, []);

  const selectedNode = useMemo(() => {
    if (!selectedNodeId || !graph) return null;
    return graph.nodes.find((n) => n.id === selectedNodeId) ?? null;
  }, [selectedNodeId, graph]);

  const toastClass =
    toast?.kind === 'success'
      ? 'bg-green-100 text-green-800'
      : toast?.kind === 'warning'
        ? 'bg-yellow-100 text-yellow-800'
        : 'bg-red-100 text-red-800';

  return (
    <main className="min-h-screen p-6">
      <header className="mb-4">
        <h1 className="text-2xl font-bold">BT 编辑器</h1>
        <p className="text-gray-600 text-sm">Phase C.3 — 行为树可视化 + 编辑</p>
      </header>

      {/* Top bar: NPC + tree dropdowns */}
      <div className="mb-4 flex flex-wrap items-center gap-3">
        <label className="font-semibold text-sm">NPC:</label>
        <select
          className="border rounded px-3 py-2 min-w-[200px]"
          value={npcId}
          onChange={(e) => setNpcId(e.target.value)}
          data-testid="bt-npc-dropdown"
        >
          {NPC_STUB.map((n) => (
            <option key={n.id} value={n.id}>
              {n.label}
            </option>
          ))}
        </select>

        <label className="font-semibold text-sm ml-2">Tree:</label>
        <select
          className="border rounded px-3 py-2 min-w-[200px]"
          value={treeName ?? ''}
          onChange={(e) => setTreeName(e.target.value || null)}
          data-testid="bt-tree-dropdown"
        >
          <option value="">— 选择 tree —</option>
          {trees.map((t) => (
            <option key={t.name} value={t.name}>
              {t.name} (v{t.version})
            </option>
          ))}
        </select>

        {loading && <span className="text-gray-500 text-sm">加载中…</span>}
      </div>

      {error && (
        <p className="text-red-600 text-sm mb-3" data-testid="bt-error">
          {error}
        </p>
      )}

      {toast && (
        <div
          className={`fixed top-4 right-4 px-4 py-2 rounded shadow ${toastClass}`}
          data-testid="bt-toast"
          data-kind={toast.kind}
        >
          {toast.msg}
        </div>
      )}

      {/* 3-column layout */}
      <div className="grid grid-cols-12 gap-4" style={{ minHeight: 600 }}>
        {/* Monaco editor (~40%) */}
        <section className="col-span-5 border rounded p-2">
          <h2 className="text-sm font-semibold mb-2">tree_json</h2>
          <Editor
            height="540px"
            defaultLanguage="json"
            value={treeJson ? JSON.stringify(treeJson, null, 2) : ''}
            onChange={handleEditorChange}
            options={{
              minimap: { enabled: false },
              fontSize: 12,
              readOnly: !treeJson,
            }}
            data-testid="bt-monaco"
          />
        </section>

        {/* React Flow graph (~40%) */}
        <section className="col-span-5 border rounded p-2">
          <h2 className="text-sm font-semibold mb-2">Graph</h2>
          {graph ? (
            <BTGraph graph={graph} onNodeClick={setSelectedNodeId} />
          ) : (
            <p className="text-gray-500 text-sm">暂无 graph</p>
          )}
        </section>

        {/* Metadata + buttons + trace (~20%) */}
        <section className="col-span-2 border rounded p-3 space-y-3">
          <div>
            <h2 className="text-sm font-semibold">元数据</h2>
            {treeFull ? (
              <div className="text-xs space-y-1 mt-1">
                <p>version: {treeFull.version}</p>
                <p>updated: {treeFull.updated_at}</p>
                <p>nodes: {graph?.nodes.length ?? 0}</p>
                <p>edges: {graph?.edges.length ?? 0}</p>
              </div>
            ) : (
              <p className="text-xs text-gray-500 mt-1">—</p>
            )}
          </div>

          {selectedNode && (
            <div>
              <h3 className="text-sm font-semibold">选中节点</h3>
              <div className="text-xs space-y-1 mt-1">
                <p>id: {selectedNode.id}</p>
                <p>type: {selectedNode.type}</p>
                <p>label: {selectedNode.label}</p>
              </div>
            </div>
          )}

          <div className="space-y-2">
            <button
              className="w-full px-3 py-2 bg-blue-600 text-white text-sm rounded disabled:opacity-50"
              onClick={handleSave}
              disabled={!treeName || !treeJson || saving}
              data-testid="bt-save-btn"
            >
              {saving ? '保存中…' : 'Save'}
            </button>
            <button
              className="w-full px-3 py-2 bg-purple-600 text-white text-sm rounded disabled:opacity-50"
              onClick={handleSimulate}
              disabled={!treeName || !treeJson || simulating}
              data-testid="bt-simulate-btn"
            >
              {simulating ? '模拟中…' : 'Simulate'}
            </button>
          </div>

          {trace && (
            <div>
              <h3 className="text-sm font-semibold">Trace log</h3>
              <p className="text-xs text-gray-600">final: {trace.status}</p>
              <ul className="text-xs mt-1 max-h-48 overflow-y-auto">
                {trace.trace.map((t, i) => (
                  <li key={i} className="border-b py-1">
                    #{t.tick_count} {t.node_id} → {t.status}
                  </li>
                ))}
              </ul>
            </div>
          )}
        </section>
      </div>
    </main>
  );
}