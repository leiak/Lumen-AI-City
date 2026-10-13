'use client';
import { useCallback, useMemo, useState } from 'react';
import ReactFlow, {
  applyNodeChanges,
  Background,
  Connection,
  Controls,
  Edge,
  EdgeChange,
  MiniMap,
  Node,
  NodeChange,
} from 'reactflow';
import SagaStepNode, { SagaStepNodeData } from './SagaStepNode';
import { SagaGraph as SagaGraphType } from '@/lib/saga_loader';
import { layoutSagaGraph } from '@/lib/saga_layout';
import { graphToSagaYaml } from '@/lib/saga_export';
import { hasBlockingSagaIssues, validateSagaGraph } from '@/lib/saga_validation';

const nodeTypes = { sagaStep: SagaStepNode };
type EditorNode = Node<SagaStepNodeData>;

interface Props {
  graph: SagaGraphType;
}

function uniqueId(existing: string[], prefix: string): string {
  let index = existing.length;
  let id = `${prefix}_${index}`;
  while (existing.includes(id)) {
    index += 1;
    id = `${prefix}_${index}`;
  }
  return id;
}

export default function SagaGraphEditor({ graph }: Props) {
  const initial = useMemo(() => layoutSagaGraph(graph), [graph]);
  const [nodes, setNodes] = useState<EditorNode[]>(initial.nodes as EditorNode[]);
  const [edges, setEdges] = useState<Edge[]>(initial.edges);

  const editableGraph = useMemo(
    () => ({ ...graph, nodes: nodes.map((node) => ({ ...node.data, id: node.id })) , edges: edges.map((edge) => ({
      id: edge.id,
      source: edge.source,
      target: edge.target,
      kind: edge.label === 'comp' ? 'compensation' as const : 'forward' as const,
    })) }),
    [graph, nodes, edges],
  );
  const issues = useMemo(() => validateSagaGraph(editableGraph), [editableGraph]);
  const blocking = hasBlockingSagaIssues(issues);
  const selectedNode = nodes.find((node) => node.selected);

  const onNodesChange = useCallback(
    (changes: NodeChange[]) => setNodes((current) => applyNodeChanges(changes, current)),
    [],
  );

  const onEdgesChange = useCallback(
    (changes: EdgeChange[]) => setEdges((current) => {
      let next = current;
      for (const change of changes) {
        if (change.type === 'remove') next = next.filter((edge) => edge.id !== change.id);
      }
      return next;
    }),
    [],
  );

  const onConnect = useCallback((connection: Connection) => {
    if (!connection.source || !connection.target) return;
    const target = nodes.find((node) => node.id === connection.target);
    const kind = target?.data.type === 'compensation' ? 'compensation' : 'forward';
    const newEdge: Edge = {
        id: `e_${connection.source}_${connection.target}_${Date.now()}`,
        source: connection.source,
        target: connection.target,
        type: 'smoothstep',
        animated: kind === 'forward',
        label: kind === 'compensation' ? 'comp' : null,
        style: {
          stroke: kind === 'forward' ? '#16a34a' : '#ea580c',
          strokeWidth: 2,
        },
        labelStyle: kind === 'compensation' ? { fontSize: 10, fill: '#ea580c' } : undefined,
    };
    setEdges((current) => [...current, newEdge]);
  }, [nodes]);

  const addNode = useCallback((type: 'forward' | 'compensation') => {
    setNodes((current) => {
      const id = uniqueId(current.map((node) => node.id), type === 'forward' ? 'step' : 'comp');
      const data: SagaStepNodeData = {
        label: type === 'forward' ? '新步骤' : '新补偿',
        type,
        worker_id: '',
        npc_id: type === 'forward' ? '' : undefined,
        action: type === 'forward' ? 'say' : 'revert_say',
        text: type === 'forward' ? '' : undefined,
      };
      return [
        ...current,
        {
          id,
          type: 'sagaStep',
          data,
          position: { x: 80 + current.length * 20, y: 80 + current.length * 40 },
        },
      ];
    });
  }, []);

  const updateSelectedNode = useCallback((field: keyof SagaStepNodeData, value: string) => {
    setNodes((current) => current.map((node) => {
      if (!node.selected) return node;
      return { ...node, data: { ...node.data, [field]: value } };
    }));
  }, []);

  const downloadYaml = useCallback(() => {
    const yaml = graphToSagaYaml(editableGraph);
    const blob = new Blob([yaml], { type: 'application/x-yaml;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement('a');
    anchor.href = url;
    anchor.download = `${graph.saga_id || 'saga'}.yaml`;
    anchor.click();
    URL.revokeObjectURL(url);
  }, [editableGraph, graph.saga_id]);

  return (
    <div className="grid grid-cols-1 gap-4 xl:grid-cols-[1fr_340px]">
      <div className="relative">
        <div className="mb-3 flex flex-wrap items-center gap-2">
          <button type="button" className="rounded border px-3 py-1.5 text-sm hover:bg-gray-50" onClick={() => addNode('forward')}>+ Forward</button>
          <button type="button" className="rounded border px-3 py-1.5 text-sm hover:bg-gray-50" onClick={() => addNode('compensation')}>+ Compensation</button>
          <button type="button" className="rounded border border-black px-3 py-1.5 text-sm font-medium text-white bg-black hover:bg-gray-800" onClick={downloadYaml} disabled={blocking}>
            导出 YAML
          </button>
          <span className={`rounded px-2 py-1 text-sm ${blocking ? 'bg-red-100 text-red-700' : 'bg-green-100 text-green-700'}`} data-testid="saga-validation-state">
            {blocking ? '校验未通过' : '校验通过'}
          </span>
        </div>
        <div style={{ width: '100%', height: '68vh', border: '1px solid #e5e7eb', borderRadius: 8 }}>
          <ReactFlow
            nodes={nodes}
            edges={edges}
            nodeTypes={nodeTypes}
            onNodesChange={onNodesChange}
            onEdgesChange={onEdgesChange}
            onConnect={onConnect}
            fitView
            fitViewOptions={{ padding: 0.2 }}
            proOptions={{ hideAttribution: true }}
            deleteKeyCode={['Delete']}
          >
            <Background />
            <Controls />
            <MiniMap />
          </ReactFlow>
        </div>
        <p className="mt-2 text-sm text-gray-500">
          拖动节点调整布局；从节点上下圆点拖拽连线；选中节点/边后按 Delete 删除。
        </p>
      </div>

      <aside className="rounded-lg border p-4">
        <h2 className="mb-3 text-lg font-semibold">节点属性</h2>
        {selectedNode ? (
          <div className="space-y-3" data-testid="saga-node-editor">
            <label className="block text-sm">
              显示名称
              <input className="mt-1 w-full rounded border px-2 py-1" value={selectedNode.data.label} onChange={(event) => updateSelectedNode('label', event.target.value)} />
            </label>
            {selectedNode.data.type !== 'start' && selectedNode.data.type !== 'end' && (
              <label className="block text-sm">
                worker_id
                <input className="mt-1 w-full rounded border px-2 py-1" value={selectedNode.data.worker_id ?? ''} onChange={(event) => updateSelectedNode('worker_id', event.target.value)} />
              </label>
            )}
            {selectedNode.data.type === 'forward' && (
              <label className="block text-sm">
                npc_id
                <input className="mt-1 w-full rounded border px-2 py-1" value={selectedNode.data.npc_id ?? ''} onChange={(event) => updateSelectedNode('npc_id', event.target.value)} />
              </label>
            )}
            {selectedNode.data.type !== 'start' && selectedNode.data.type !== 'end' && (
              <label className="block text-sm">
                action
                <input className="mt-1 w-full rounded border px-2 py-1" value={selectedNode.data.action ?? ''} onChange={(event) => updateSelectedNode('action', event.target.value)} />
              </label>
            )}
            {selectedNode.data.type === 'forward' && (
              <label className="block text-sm">
                payload.text
                <textarea className="mt-1 h-24 w-full rounded border px-2 py-1" value={selectedNode.data.text ?? ''} onChange={(event) => updateSelectedNode('text', event.target.value)} />
              </label>
            )}
          </div>
        ) : (
          <p className="text-sm text-gray-500">点击一个节点后可编辑属性。</p>
        )}

        <h2 className="mb-2 mt-5 text-lg font-semibold">校验结果</h2>
        {issues.length === 0 ? (
          <p className="rounded bg-green-50 p-2 text-sm text-green-700">没有发现问题。</p>
        ) : (
          <ul className="max-h-64 space-y-2 overflow-auto text-sm" data-testid="saga-validation-issues">
            {issues.map((issue, index) => (
              <li key={`${issue.path}-${index}`} className={`rounded p-2 ${issue.severity === 'error' ? 'bg-red-50 text-red-700' : 'bg-amber-50 text-amber-700'}`}>
                <strong>{issue.severity === 'error' ? '错误' : '警告'}:</strong> {issue.message}
              </li>
            ))}
          </ul>
        )}
      </aside>
    </div>
  );
}
