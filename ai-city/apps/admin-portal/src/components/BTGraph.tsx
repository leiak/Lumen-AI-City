'use client';
import { useMemo } from 'react';
import ReactFlow, { Background, Controls, MiniMap, Node, Edge } from 'reactflow';
import 'reactflow/dist/style.css';
import BTNode from './BTNode';
import type { BTGraph as BTGraphType } from '@/lib/bt_loader';
import { layoutBTGraph } from '@/lib/bt_layout';

const nodeTypes = { btNode: BTNode };

interface Props {
  graph: BTGraphType;
  onNodeClick?: (nodeId: string) => void;
}

export default function BTGraph({ graph, onNodeClick }: Props) {
  const laid: { nodes: Node[]; edges: Edge[] } = useMemo(
    () => layoutBTGraph(graph),
    [graph],
  );

  return (
    <div
      style={{
        width: '100%',
        height: '100%',
        minHeight: 480,
        border: '1px solid #e5e7eb',
        borderRadius: 8,
      }}
      data-testid="bt-graph-container"
    >
      <ReactFlow
        nodes={laid.nodes}
        edges={laid.edges}
        nodeTypes={nodeTypes}
        fitView
        fitViewOptions={{ padding: 0.2 }}
        proOptions={{ hideAttribution: true }}
        onNodeClick={(_e: React.MouseEvent, n: Node) => onNodeClick?.(n.id)}
      >
        <Background />
        <Controls />
        <MiniMap />
      </ReactFlow>
    </div>
  );
}