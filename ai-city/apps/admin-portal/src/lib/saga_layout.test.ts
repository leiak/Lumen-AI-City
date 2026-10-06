import { describe, it, expect } from 'vitest';
import { parseSagaToGraph } from './saga_loader';
import { layoutSagaGraph } from './saga_layout';

const WELCOME_3NPC = `saga_id: welcome_3npc
description: 3 NPC 协作欢迎新玩家
steps:
  - worker_id: worker_a
    npc_id: npc_a_wang_boss
    action: say
    payload: { text: "欢迎来到 A 城！" }
  - worker_id: worker_b
    npc_id: npc_b_grace_healer
    action: say
    payload: { text: "我是护士 grace，欢迎你！" }
  - worker_id: worker_c
    npc_id: npc_a_book_keeper
    action: say
    payload: { text: "书店在城东，随时来坐坐。" }
compensation:
  - worker_id: worker_a
    action: revert_say
  - worker_id: worker_b
    action: revert_say
  - worker_id: worker_c
    action: revert_say
`;

describe('layoutSagaGraph', () => {
  it('returns nodes with numeric positions for welcome_3npc', () => {
    const graph = parseSagaToGraph(WELCOME_3NPC);
    const { nodes, edges } = layoutSagaGraph(graph);

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

  it('every node has type=sagaStep and propagates SagaNode data', () => {
    const graph = parseSagaToGraph(WELCOME_3NPC);
    const { nodes } = layoutSagaGraph(graph);

    for (const n of nodes) {
      expect(n.type).toBe('sagaStep');
      expect(n.data.label).toBeTruthy();
      expect(['forward', 'compensation', 'start', 'end']).toContain(n.data.type);
    }
  });

  it('forward edges are green (animated); compensation edges are orange (labelled)', () => {
    const graph = parseSagaToGraph(WELCOME_3NPC);
    const { edges } = layoutSagaGraph(graph);

    for (const e of edges) {
      const srcNode = graph.nodes.find((n) => n.id === e.source);
      const tgtNode = graph.nodes.find((n) => n.id === e.target);
      const isForward =
        (srcNode?.id === 'start' && tgtNode?.id.startsWith('step_')) ||
        (srcNode?.id.startsWith('step_') && tgtNode?.id.startsWith('step_')) ||
        (srcNode?.id.startsWith('step_') && tgtNode?.id === 'end');
      const isCompensation =
        srcNode?.id.startsWith('step_') && tgtNode?.id.startsWith('comp_');

      if (isForward) {
        expect(e.animated).toBe(true);
        expect(e.style?.stroke).toBe('#16a34a');
      } else if (isCompensation) {
        expect(e.style?.stroke).toBe('#ea580c');
        expect(e.label).toBe('comp');
      }
    }
  });

  it('handles saga without compensation — no comp edges in layout', () => {
    const simpleYaml = `saga_id: simple
description: forward only
steps:
  - worker_id: worker_a
    action: say
    payload: { text: "hi" }
`;
    const graph = parseSagaToGraph(simpleYaml);
    const { nodes, edges } = layoutSagaGraph(graph);

    expect(nodes.length).toBe(3); // start, step_0, end
    expect(edges.length).toBe(2);
    expect(edges.every((e) => e.animated === true)).toBe(true);
  });
});
