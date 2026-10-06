import dagre from '@dagrejs/dagre';
import { Edge, Node } from 'reactflow';
import { SagaGraph } from './saga_loader';

const NODE_WIDTH = 200;
const NODE_HEIGHT = 80;

/** Lay out SagaGraph nodes with dagre. Returns React Flow nodes/edges with positions. */
export function layoutSagaGraph(graph: SagaGraph): { nodes: Node[]; edges: Edge[] } {
  const g = new dagre.graphlib.Graph();
  g.setDefaultEdgeLabel(() => ({}));
  g.setGraph({ rankdir: 'TB', nodesep: 30, ranksep: 60 });

  graph.nodes.forEach((n) => {
    g.setNode(n.id, { width: NODE_WIDTH, height: NODE_HEIGHT });
  });
  graph.edges.forEach((e) => {
    g.setEdge(e.source, e.target);
  });

  dagre.layout(g);

  const nodes: Node[] = graph.nodes.map((n) => {
    const pos = g.node(n.id);
    return {
      id: n.id,
      type: 'sagaStep',
      data: {
        label: n.label,
        type: n.type,
        worker_id: n.worker_id,
        npc_id: n.npc_id,
        action: n.action,
        text: n.text,
      },
      position: {
        x: (pos?.x ?? 0) - NODE_WIDTH / 2,
        y: (pos?.y ?? 0) - NODE_HEIGHT / 2,
      },
    };
  });

  const edges: Edge[] = graph.edges.map((e) => ({
    id: e.id,
    source: e.source,
    target: e.target,
    type: 'smoothstep',
    animated: e.kind === 'forward',
    style: {
      stroke: e.kind === 'forward' ? '#16a34a' : '#ea580c',
      strokeWidth: 2,
    },
    label: e.kind === 'compensation' ? 'comp' : null,
    labelStyle: { fontSize: 10, fill: '#ea580c' },
  }));

  return { nodes, edges };
}
