import { describe, it, expect } from 'vitest';
import { parseBtTreeToGraph, type BTJSON } from './bt_loader';
import { layoutBTGraph } from './bt_layout';

const SIMPLE_TREE: BTJSON = {
  version: '1.0.0',
  root: {
    id: 'root',
    type: 'sequence',
    children: [
      { id: 'c1', type: 'condition', expression: 'hp < 50' },
      { id: 'c2', type: 'action', action: 'say', args: { text: 'help' } },
      {
        id: 'c3',
        type: 'selector',
        children: [
          { id: 'a1', type: 'action', action: 'give_item' },
          { id: 'a2', type: 'action', action: 'wait' },
        ],
      },
    ],
  },
};

describe('layoutBTGraph', () => {
  it('returns nodes with numeric positions (no NaN) for a multi-level tree', () => {
    const graph = parseBtTreeToGraph(SIMPLE_TREE);
    const { nodes, edges } = layoutBTGraph(graph);

    expect(nodes.length).toBe(graph.nodes.length);
    expect(edges.length).toBe(graph.edges.length);

    for (const n of nodes) {
      expect(typeof n.position.x).toBe('number');
      expect(typeof n.position.y).toBe('number');
      expect(Number.isFinite(n.position.x)).toBe(true);
      expect(Number.isFinite(n.position.y)).toBe(true);
      expect(Number.isNaN(n.position.x)).toBe(false);
      expect(Number.isNaN(n.position.y)).toBe(false);
    }
  });

  it('every node has type=btNode and a data.nodeType in the 7 canonical BT types', () => {
    const graph = parseBtTreeToGraph(SIMPLE_TREE);
    const { nodes } = layoutBTGraph(graph);

    const validTypes = new Set([
      'sequence',
      'selector',
      'action',
      'condition',
      'decorator',
      'subtree',
      'llm',
    ]);

    for (const n of nodes) {
      expect(n.type).toBe('btNode');
      const data = n.data as { nodeType: string; label: string; node: { id: string } };
      expect(validTypes.has(data.nodeType)).toBe(true);
      expect(typeof data.label).toBe('string');
      expect(typeof data.node.id).toBe('string');
    }
  });

  it('every edge is a smoothstep with valid source/target references', () => {
    const graph = parseBtTreeToGraph(SIMPLE_TREE);
    const { edges, nodes } = layoutBTGraph(graph);

    const ids = new Set(nodes.map((n) => n.id));
    for (const e of edges) {
      expect(e.type).toBe('smoothstep');
      expect(ids.has(e.source)).toBe(true);
      expect(ids.has(e.target)).toBe(true);
      expect(typeof e.style?.stroke).toBe('string');
    }
  });
});
