import { describe, expect, it } from 'vitest';
import { SagaGraph } from './saga_loader';
import { hasBlockingSagaIssues, validateSagaGraph } from './saga_validation';

const validGraph: SagaGraph = {
  saga_id: 'valid',
  description: '',
  nodes: [
    { id: 'start', label: 'start', type: 'start' },
    { id: 'step_0', label: 'step', type: 'forward', worker_id: 'w', action: 'say' },
    { id: 'comp_0', label: 'comp', type: 'compensation', worker_id: 'w', action: 'revert' },
    { id: 'end', label: 'end', type: 'end' },
  ],
  edges: [
    { id: 'e1', source: 'start', target: 'step_0', kind: 'forward' },
    { id: 'e2', source: 'step_0', target: 'end', kind: 'forward' },
    { id: 'e3', source: 'step_0', target: 'comp_0', kind: 'compensation' },
  ],
};

describe('validateSagaGraph', () => {
  it('accepts a connected saga with compensation', () => {
    expect(validateSagaGraph(validGraph)).toEqual([]);
  });

  it('reports duplicated node ids', () => {
    const graph: SagaGraph = {
      ...validGraph,
      nodes: [...validGraph.nodes, { id: 'step_0', label: 'duplicate', type: 'forward' }],
    };
    const messages = validateSagaGraph(graph).map((issue) => issue.message).join(' ');
    expect(messages).toContain('节点 ID 重复');
  });

  it('reports forward steps that cannot close at end', () => {
    const graph: SagaGraph = {
      ...validGraph,
      nodes: [validGraph.nodes[0], validGraph.nodes[1], validGraph.nodes[3]],
      edges: [validGraph.edges[0], validGraph.edges[2]],
    };
    const issues = validateSagaGraph(graph);
    expect(issues.map((issue) => issue.message).join(' ')).toContain('无法到达 end');
    expect(hasBlockingSagaIssues(issues)).toBe(true);
  });

  it('rejects a forward cycle', () => {
    const graph: SagaGraph = {
      saga_id: 'cycle',
      description: '',
      nodes: [
        { id: 'start', label: 'start', type: 'start' },
        { id: 'step_0', label: 'A', type: 'forward' },
        { id: 'step_1', label: 'B', type: 'forward' },
        { id: 'end', label: 'end', type: 'end' },
      ],
      edges: [
        { id: 'e1', source: 'start', target: 'step_0', kind: 'forward' },
        { id: 'e2', source: 'step_0', target: 'step_1', kind: 'forward' },
        { id: 'e3', source: 'step_1', target: 'step_0', kind: 'forward' },
        { id: 'e4', source: 'step_1', target: 'end', kind: 'forward' },
      ],
    };
    expect(validateSagaGraph(graph).some((issue) => issue.message.includes('循环'))).toBe(true);
  });
});
