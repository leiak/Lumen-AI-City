import dagre from '@dagrejs/dagre';
import { Edge, Node } from 'reactflow';
import { BTGraph, BTNode, BTNodeType } from './bt_loader';

const NODE_WIDTH = 220;
const NODE_HEIGHT = 90;

/** Per-7-BT-type colours used by both the layout (edge colour) and
 *  BTNode.tsx (background). Keep in sync with src/components/BTNode.tsx. */
export const BT_NODE_COLORS: Record<BTNodeType, { bg: string; border: string }> = {
  sequence: { bg: '#e0e7ff', border: '#4f46e5' }, // indigo
  selector: { bg: '#f3e8ff', border: '#9333ea' }, // purple
  action: { bg: '#dcfce7', border: '#16a34a' }, // green
  condition: { bg: '#ffedd5', border: '#ea580c' }, // orange
  decorator: { bg: '#fce7f3', border: '#db2777' }, // pink
  subtree: { bg: '#ccfbf1', border: '#0d9488' }, // teal
  llm: { bg: '#fef9c3', border: '#a16207' }, // yellow
};

/** Map internal BT node → BTNode component data shape.
 *
 *  BTNode.tsx expects a nested `node` object with shape:
 *    { id, type, name?, kind?, tree_id?, args?: unknown[],
 *      prompt?, expected?, expression? }
 *  plus a top-level `nodeType: BTNodeType` and a `label: string`.
 */
function toBTNodeData(n: BTNode): {
  label: string;
  nodeType: BTNodeType;
  node: Record<string, unknown>;
} {
  const node: Record<string, unknown> = {
    id: n.id,
    type: n.type,
  };
  if (n.name !== undefined) node.name = n.name;
  if (n.action !== undefined) node.name = n.name ?? n.action;
  // args: present schema uses array; coerce object → array of [k,v] pairs
  if (n.args !== undefined) {
    node.args = Array.isArray(n.args)
      ? n.args
      : Object.entries(n.args);
  }
  // decorator: name "kind" in the BTNode component
  if (n.decorator !== undefined) node.kind = n.decorator;
  // subtree: name "tree_id" in the BTNode component
  if (n.ref !== undefined) node.tree_id = n.ref;
  // condition
  if (n.expression !== undefined) node.expression = n.expression;
  // llm: prompt_template → prompt
  if (n.prompt_template !== undefined) node.prompt = n.prompt_template;
  // llm.expected — passthrough if present on raw input
  const rawExpected = (n as unknown as { expected?: string }).expected;
  if (rawExpected !== undefined) node.expected = rawExpected;

  return { label: n.label, nodeType: n.type, node };
}

/** Lay out a BT graph with dagre (TB). Returns React Flow nodes/edges
 *  shaped to be consumed by src/components/BTNode.tsx.
 */
export function layoutBTGraph(graph: BTGraph): {
  nodes: Node[];
  edges: Edge[];
} {
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
      type: 'btNode',
      data: toBTNodeData(n),
      position: {
        x: (pos?.x ?? 0) - NODE_WIDTH / 2,
        y: (pos?.y ?? 0) - NODE_HEIGHT / 2,
      },
    };
  });

  const edges: Edge[] = graph.edges.map((e) => {
    const src = graph.nodes.find((n) => n.id === e.source);
    const color = src ? BT_NODE_COLORS[src.type].border : '#6b7280';
    return {
      id: e.id,
      source: e.source,
      target: e.target,
      type: 'smoothstep',
      style: { stroke: color, strokeWidth: 2 },
    };
  });

  return { nodes, edges };
}
