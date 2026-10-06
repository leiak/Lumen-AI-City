import { describe, it, expect } from 'vitest';
import { parseSagaToGraph } from './saga_loader';

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

describe('parseSagaToGraph', () => {
  it('parses welcome_3npc — produces start + 3 forward steps + 3 comps + end', () => {
    const g = parseSagaToGraph(WELCOME_3NPC);
    expect(g.saga_id).toBe('welcome_3npc');
    expect(g.description).toBe('3 NPC 协作欢迎新玩家');

    const byId = new Map(g.nodes.map((n) => [n.id, n]));
    expect(byId.get('start')?.type).toBe('start');
    expect(byId.get('end')?.type).toBe('end');
    expect(byId.get('step_0')?.type).toBe('forward');
    expect(byId.get('step_1')?.type).toBe('forward');
    expect(byId.get('step_2')?.type).toBe('forward');
    expect(byId.get('comp_0')?.type).toBe('compensation');
    expect(byId.get('comp_1')?.type).toBe('compensation');
    expect(byId.get('comp_2')?.type).toBe('compensation');
  });

  it('nodes count matches: 1 start + N steps + N comps + 1 end', () => {
    const g = parseSagaToGraph(WELCOME_3NPC);
    // 1 start + 3 forward + 3 compensation + 1 end = 8
    expect(g.nodes.length).toBe(8);
  });

  it('forward edges chain: start -> step[0] -> step[1] -> ... -> step[N-1] -> end', () => {
    const g = parseSagaToGraph(WELCOME_3NPC);

    const forwardEdges = g.edges.filter((e) => e.kind === 'forward');
    expect(forwardEdges.length).toBe(4); // start->s0, s0->s1, s1->s2, s2->end

    const chain = forwardEdges.map((e) => `${e.source}->${e.target}`);
    expect(chain).toEqual([
      'start->step_0',
      'step_0->step_1',
      'step_1->step_2',
      'step_2->end',
    ]);
  });

  it('compensation edges: step[i] -> comp[i]', () => {
    const g = parseSagaToGraph(WELCOME_3NPC);

    const compEdges = g.edges.filter((e) => e.kind === 'compensation');
    expect(compEdges.length).toBe(3);

    const pairs = compEdges.map((e) => `${e.source}->${e.target}`);
    expect(pairs).toEqual([
      'step_0->comp_0',
      'step_1->comp_1',
      'step_2->comp_2',
    ]);
  });

  it('edge kinds: forward edges have kind=forward; compensation edges have kind=compensation', () => {
    const g = parseSagaToGraph(WELCOME_3NPC);

    for (const e of g.edges) {
      if (e.source === 'start' || e.target === 'end' || e.source.startsWith('step_') && e.target.startsWith('step_')) {
        expect(e.kind).toBe('forward');
      } else if (e.source.startsWith('step_') && e.target.startsWith('comp_')) {
        expect(e.kind).toBe('compensation');
      }
    }
  });

  it('throws on missing saga_id', () => {
    const yaml = `description: missing id
steps:
  - worker_id: worker_a
    action: say
`;
    expect(() => parseSagaToGraph(yaml)).toThrow(/saga_id/);
  });

  it('throws on missing steps', () => {
    const yaml = `saga_id: no_steps
description: no steps here
`;
    expect(() => parseSagaToGraph(yaml)).toThrow(/steps/);
  });

  it('handles no compensation — no comp nodes/edges', () => {
    const yaml = `saga_id: simple
description: forward only
steps:
  - worker_id: worker_a
    action: say
    payload: { text: "hi" }
`;
    const g = parseSagaToGraph(yaml);

    // 1 start + 1 forward + 1 end = 3 nodes
    expect(g.nodes.length).toBe(3);
    expect(g.nodes.find((n) => n.id === 'start')).toBeTruthy();
    expect(g.nodes.find((n) => n.id === 'end')).toBeTruthy();
    expect(g.nodes.find((n) => n.id === 'step_0')).toBeTruthy();
    expect(g.nodes.find((n) => n.type === 'compensation')).toBeUndefined();

    // 2 forward edges (start->step_0, step_0->end); 0 compensation edges
    expect(g.edges.length).toBe(2);
    expect(g.edges.every((e) => e.kind === 'forward')).toBe(true);
  });
});
