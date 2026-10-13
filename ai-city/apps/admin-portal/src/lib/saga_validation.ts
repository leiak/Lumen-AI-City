import { SagaGraph, SagaNode } from './saga_loader';

export interface SagaValidationIssue {
  severity: 'error' | 'warning';
  path: string;
  message: string;
}

function reachable(ids: string[], edges: Array<{ source: string; target: string }>): Set<string> {
  const adjacency = new Map<string, string[]>();
  for (const id of ids) adjacency.set(id, []);
  for (const edge of edges) adjacency.get(edge.source)?.push(edge.target);

  const visited = new Set<string>();
  const stack = [...ids];
  while (stack.length > 0) {
    const id = stack.pop()!;
    if (visited.has(id)) continue;
    visited.add(id);
    for (const next of adjacency.get(id) ?? []) {
      if (!visited.has(next)) stack.push(next);
    }
  }
  return visited;
}

function hasCycle(ids: string[], edges: Array<{ source: string; target: string }>): boolean {
  const adjacency = new Map<string, string[]>();
  for (const id of ids) adjacency.set(id, []);
  for (const edge of edges) adjacency.get(edge.source)?.push(edge.target);

  const visiting = new Set<string>();
  const visited = new Set<string>();

  const visit = (id: string): boolean => {
    if (visiting.has(id)) return true;
    if (visited.has(id)) return false;
    visiting.add(id);
    for (const next of adjacency.get(id) ?? []) {
      if (visit(next)) return true;
    }
    visiting.delete(id);
    visited.add(id);
    return false;
  };

  return ids.some(visit);
}

function canReachEnd(
  sourceId: string,
  endId: string,
  forwardEdges: Array<{ source: string; target: string }>,
): boolean {
  const adjacency = new Map<string, string[]>();
  for (const edge of forwardEdges) {
    adjacency.set(edge.source, [...(adjacency.get(edge.source) ?? []), edge.target]);
  }

  const stack = [sourceId];
  const visited = new Set<string>();
  while (stack.length > 0) {
    const id = stack.pop()!;
    if (id === endId) return true;
    if (visited.has(id)) continue;
    visited.add(id);
    for (const next of adjacency.get(id) ?? []) stack.push(next);
  }
  return false;
}

export function validateSagaGraph(graph: SagaGraph): SagaValidationIssue[] {
  const issues: SagaValidationIssue[] = [];
  const nodes = graph.nodes ?? [];
  const edges = graph.edges ?? [];
  const byId = new Map<string, SagaNode>();

  if (!graph.saga_id?.trim()) {
    issues.push({ severity: 'error', path: 'saga_id', message: 'saga_id 不能为空' });
  }

  for (const node of nodes) {
    if (!node.id?.trim()) {
      issues.push({ severity: 'error', path: 'nodes', message: '存在没有 ID 的节点' });
      continue;
    }
    if (byId.has(node.id)) {
      issues.push({ severity: 'error', path: `nodes.${node.id}`, message: `节点 ID 重复：${node.id}` });
      continue;
    }
    byId.set(node.id, node);
    if (!node.label?.trim()) {
      issues.push({ severity: 'warning', path: `nodes.${node.id}.label`, message: `节点 ${node.id} 缺少显示名称` });
    }
  }

  const startCount = nodes.filter((n) => n.type === 'start').length;
  const endCount = nodes.filter((n) => n.type === 'end').length;
  const forwardNodes = nodes.filter((n) => n.type === 'forward');
  const compensationNodes = nodes.filter((n) => n.type === 'compensation');

  if (startCount !== 1) {
    issues.push({ severity: 'error', path: 'nodes.start', message: `start 节点必须恰好 1 个（当前 ${startCount} 个）` });
  }
  if (endCount !== 1) {
    issues.push({ severity: 'error', path: 'nodes.end', message: `end 节点必须恰好 1 个（当前 ${endCount} 个）` });
  }
  if (forwardNodes.length === 0) {
    issues.push({ severity: 'error', path: 'nodes', message: '至少需要一个 forward 步骤' });
  }

  const seenEdgePairs = new Set<string>();
  for (const edge of edges) {
    const source = byId.get(edge.source);
    const target = byId.get(edge.target);
    if (!source || !target) {
      issues.push({
        severity: 'error',
        path: `edges.${edge.id}`,
        message: `边 ${edge.id} 引用了不存在的节点：${!source ? edge.source : edge.target}`,
      });
      continue;
    }
    if (edge.source === edge.target) {
      issues.push({ severity: 'error', path: `edges.${edge.id}`, message: '边不能连接自身' });
    }

    const pairKey = `${edge.source}->${edge.target}:${edge.kind}`;
    if (seenEdgePairs.has(pairKey)) {
      issues.push({ severity: 'error', path: `edges.${edge.id}`, message: `重复边：${edge.source} → ${edge.target}` });
    }
    seenEdgePairs.add(pairKey);

    if (edge.kind === 'forward') {
      const validSource = source.type === 'start' || source.type === 'forward';
      const validTarget = target.type === 'forward' || target.type === 'end';
      if (!validSource || !validTarget) {
        issues.push({
          severity: 'error',
          path: `edges.${edge.id}`,
          message: 'forward 边只能是 start/forward → forward/end',
        });
      }
    } else if (edge.kind === 'compensation') {
      if (source.type !== 'forward' || target.type !== 'compensation') {
        issues.push({
          severity: 'error',
          path: `edges.${edge.id}`,
          message: 'compensation 边只能是 forward → compensation',
        });
      }
    } else {
      issues.push({ severity: 'error', path: `edges.${edge.id}`, message: `未知边类型：${edge.kind}` });
    }
  }

  const start = nodes.find((n) => n.type === 'start');
  const end = nodes.find((n) => n.type === 'end');
  const forwardIds = forwardNodes.map((n) => n.id);
  const forwardEdges = edges.filter((e) => e.kind === 'forward');

  if (start && forwardIds.length > 0) {
    const reached = reachable([start.id], forwardEdges);
    for (const node of forwardNodes) {
      if (!reached.has(node.id)) {
        issues.push({ severity: 'error', path: `nodes.${node.id}`, message: `步骤 ${node.id} 未从 start 可达` });
      }
    }
  }

  if (end) {
    for (const node of forwardNodes) {
      if (!canReachEnd(node.id, end.id, forwardEdges)) {
        issues.push({ severity: 'error', path: `nodes.${node.id}`, message: `步骤 ${node.id} 无法到达 end` });
      }
    }
  }

  if (hasCycle(forwardIds, forwardEdges.filter((e) => forwardIds.includes(e.source) && forwardIds.includes(e.target)))) {
    issues.push({ severity: 'error', path: 'edges', message: 'forward 流程不允许出现循环' });
  }

  const compensatedTargets = new Set(
    edges.filter((e) => e.kind === 'compensation').map((e) => e.target),
  );
  for (const node of compensationNodes) {
    if (!compensatedTargets.has(node.id)) {
      issues.push({
        severity: 'warning',
        path: `nodes.${node.id}`,
        message: `补偿节点 ${node.id} 没有关联到任何 forward 步骤`,
      });
    }
    if (!node.worker_id?.trim() || !node.action?.trim()) {
      issues.push({
        severity: 'warning',
        path: `nodes.${node.id}`,
        message: `补偿节点 ${node.id} 建议填写 worker_id 和 action`,
      });
    }
  }

  return issues;
}

export function hasBlockingSagaIssues(issues: SagaValidationIssue[]): boolean {
  return issues.some((issue) => issue.severity === 'error');
}
