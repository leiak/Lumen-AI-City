import { describe, expect, it } from 'vitest';
import { parseSagaToGraph } from './saga_loader';
import { graphToSagaYaml } from './saga_export';

const YAML_SOURCE = `saga_id: export_test
description: round trip
steps:
  - worker_id: worker_a
    npc_id: npc_a
    action: say
    payload: { text: "hello" }
  - worker_id: worker_b
    action: say
compensation:
  - worker_id: worker_a
    action: revert_say
`;

describe('graphToSagaYaml', () => {
  it('round-trips forward order and paired compensation', () => {
    const graph = parseSagaToGraph(YAML_SOURCE);
    const exported = graphToSagaYaml(graph);
    const reparsed = parseSagaToGraph(exported);

    expect(reparsed.saga_id).toBe('export_test');
    expect(reparsed.description).toBe('round trip');
    expect(reparsed.nodes.filter((node) => node.type === 'forward').map((node) => node.worker_id))
      .toEqual(['worker_a', 'worker_b']);
    expect(reparsed.nodes.filter((node) => node.type === 'compensation').map((node) => node.action))
      .toEqual(['revert_say']);
  });
});
