import yaml from 'js-yaml';
import { SagaGraph, SagaNode } from './saga_loader';

function orderedForwardNodes(graph: SagaGraph): SagaNode[] {
  const byId = new Map(graph.nodes.map((node) => [node.id, node]));
  const forward = graph.edges
    .filter((edge) => edge.kind === 'forward')
    .map((edge) => ({ source: edge.source, target: edge.target }));
  const start = graph.nodes.find((node) => node.type === 'start');
  if (!start) return graph.nodes.filter((node) => node.type === 'forward');

  const adjacency = new Map<string, string[]>();
  for (const edge of forward) {
    adjacency.set(edge.source, [...(adjacency.get(edge.source) ?? []), edge.target]);
  }

  const ordered: SagaNode[] = [];
  const visited = new Set<string>();
  const stack = [...(adjacency.get(start.id) ?? [])].reverse();
  while (stack.length > 0) {
    const id = stack.pop()!;
    const node = byId.get(id);
    if (!node || node.type !== 'forward' || visited.has(id)) continue;
    visited.add(id);
    ordered.push(node);
    stack.push(...[...(adjacency.get(id) ?? [])].reverse());
  }

  for (const node of graph.nodes) {
    if (node.type === 'forward' && !visited.has(node.id)) ordered.push(node);
  }
  return ordered;
}

export function graphToSagaYaml(graph: SagaGraph): string {
  const steps = orderedForwardNodes(graph).map((node) => {
    const step: Record<string, unknown> = {
      worker_id: node.worker_id ?? node.id,
    };
    if (node.npc_id) step.npc_id = node.npc_id;
    step.action = node.action ?? 'say';
    if (node.text) step.payload = { text: node.text };
    return step;
  });

  const compensationByStep = new Map<string, SagaNode>();
  for (const edge of graph.edges) {
    if (edge.kind !== 'compensation') continue;
    const node = graph.nodes.find((item) => item.id === edge.target);
    if (node && node.type === 'compensation' && !compensationByStep.has(edge.source)) {
      compensationByStep.set(edge.source, node);
    }
  }

  const compensation = orderedForwardNodes(graph)
    .map((step) => compensationByStep.get(step.id))
    .filter((node): node is SagaNode => Boolean(node?.worker_id && node.action))
    .map((node) => ({
      worker_id: node.worker_id,
      action: node.action,
    }));

  return yaml.dump(
    {
      saga_id: graph.saga_id,
      description: graph.description,
      steps,
      ...(compensation.length > 0 ? { compensation } : {}),
    },
    { lineWidth: 120 },
  );
}
