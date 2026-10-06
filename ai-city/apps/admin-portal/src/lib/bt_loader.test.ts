import { describe, it, expect } from 'vitest';
import { parseBtTreeToGraph, type BTJSON } from './bt_loader';

/** Minimal BT tree: root sequence with 2 children (condition + action). */
const SIMPLE_TREE: BTJSON = {
  version: '1.0.0',
  root: {
    id: 'root',
    type: 'sequence',
    name: 'Greeting',
    children: [
      {
        id: 'c1',
        type: 'condition',
        expression: 'player.distance_to(npc) < 10',
      },
      {
        id: 'c2',
        type: 'action',
        action: 'say',
        args: { text: '欢迎来到酒馆！' },
      },
    ],
  },
};

describe('parseBtTreeToGraph', () => {
  it('parses simple sequence tree → 3 nodes + 2 edges', () => {
    const g = parseBtTreeToGraph(SIMPLE_TREE);

    expect(g.rootId).toBe('root');
    expect(g.nodes.length).toBe(3);
    expect(g.edges.length).toBe(2);

    const byId = new Map(g.nodes.map((n) => [n.id, n]));
    expect(byId.get('root')?.type).toBe('sequence');
    expect(byId.get('c1')?.type).toBe('condition');
    expect(byId.get('c2')?.type).toBe('action');

    // parent → child edges (2)
    const edgePairs = g.edges.map((e) => `${e.source}->${e.target}`).sort();
    expect(edgePairs).toEqual(['root->c1', 'root->c2']);
  });

  it('walks nested children recursively — selector containing action + LLM', () => {
    const tree: BTJSON = {
      version: '1.0.0',
      root: {
        id: 'r',
        type: 'selector',
        children: [
          {
            id: 'a1',
            type: 'action',
            action: 'move',
            args: { x: 1, y: 2 },
          },
          {
            id: 'llm1',
            type: 'llm',
            prompt_template: '说出欢迎词',
            model: 'claude-sonnet-4-6',
            max_tokens: 256,
          },
        ],
      },
    };
    const g = parseBtTreeToGraph(tree);

    expect(g.nodes.length).toBe(3);
    const llm = g.nodes.find((n) => n.id === 'llm1');
    expect(llm?.type).toBe('llm');
    expect(llm?.prompt_template).toBe('说出欢迎词');
    expect(llm?.model).toBe('claude-sonnet-4-6');

    // a1 should carry action + args
    const a1 = g.nodes.find((n) => n.id === 'a1');
    expect(a1?.action).toBe('move');
    expect(a1?.args).toEqual({ x: 1, y: 2 });
  });

  it('normalizes PascalCase types (Sequence/Condition/Action/...) to lowercase', () => {
    const tree: BTJSON = {
      version: '1.0.0',
      root: {
        id: 'r',
        type: 'Sequence', // PascalCase
        children: [
          { id: 'c', type: 'Condition' },
          { id: 'a', type: 'Action' },
          { id: 'l', type: 'LLM' },
          { id: 'd', type: 'Decorator' },
          { id: 's', type: 'SubTree' },
          { id: 'sel', type: 'Selector' },
        ],
      },
    };
    const g = parseBtTreeToGraph(tree);
    const types = new Set(g.nodes.map((n) => n.type));
    for (const t of types) {
      expect(t).toBe(t.toLowerCase());
    }
    // every canonical 7-type should appear exactly once
    expect(types).toEqual(
      new Set([
        'sequence',
        'condition',
        'action',
        'llm',
        'decorator',
        'subtree',
        'selector',
      ]),
    );
  });

  it('preserves decorator single-child relationship (one edge per decorator)', () => {
    const tree: BTJSON = {
      version: '1.0.0',
      root: {
        id: 'd1',
        type: 'decorator',
        decorator: 'Inverter',
        child: { id: 'a1', type: 'action', action: 'say' },
      },
    };
    const g = parseBtTreeToGraph(tree);

    expect(g.nodes.length).toBe(2);
    const decorChildEdge = g.edges.filter(
      (e) => e.source === 'd1' && e.target === 'a1',
    );
    expect(decorChildEdge.length).toBe(1);
  });

  it('throws on missing root', () => {
    expect(() =>
      parseBtTreeToGraph({ version: '1.0.0' } as unknown as BTJSON),
    ).toThrow(/root/);
  });

  it('throws on missing root.id', () => {
    expect(() =>
      parseBtTreeToGraph({
        version: '1.0.0',
        root: { type: 'sequence' } as unknown as BTJSON['root'],
      }),
    ).toThrow(/id/);
  });

  it('throws on unknown node type', () => {
    const tree = {
      version: '1.0.0',
      root: { id: 'r', type: 'totally_bogus' },
    } as unknown as BTJSON;
    expect(() => parseBtTreeToGraph(tree)).toThrow(/type/);
  });
});
