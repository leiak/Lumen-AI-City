'use client';
import { useMemo } from 'react';
import ReactFlow, { Background, Controls, MiniMap, Node, Edge } from 'reactflow';
import 'reactflow/dist/style.css';
import SagaStepNode from './SagaStepNode';
import { SagaGraph as SagaGraphType } from '@/lib/saga_loader';
import { layoutSagaGraph } from '@/lib/saga_layout';

const nodeTypes = { sagaStep: SagaStepNode };

interface Props {
  graph: SagaGraphType;
}

export default function SagaGraph({ graph }: Props) {
  const { nodes, edges } = useMemo(() => layoutSagaGraph(graph), [graph]);

  return (
    <div style={{ width: '100%', height: '70vh', border: '1px solid #e5e7eb', borderRadius: 8 }}>
      <ReactFlow
        nodes={nodes as Node[]}
        edges={edges as Edge[]}
        nodeTypes={nodeTypes}
        fitView
        fitViewOptions={{ padding: 0.2 }}
        proOptions={{ hideAttribution: true }}
      >
        <Background />
        <Controls />
        <MiniMap />
      </ReactFlow>
    </div>
  );
}
