import yaml from 'js-yaml';

/** Saga graph types — exported for use by components */

export type SagaStepType = 'forward' | 'compensation' | 'start' | 'end';

export interface SagaNode {
  id: string;             // unique node id (e.g. "step_0", "comp_0", "start", "end")
  label: string;          // display label
  type: SagaStepType;      // node type for color/position
  worker_id?: string;
  npc_id?: string;
  action?: string;
  text?: string;          // for forward say steps, the payload.text
}

export interface SagaEdge {
  id: string;             // unique edge id (e.g. "e_step_0_comp_0")
  source: string;         // source node id
  target: string;         // target node id
  kind: 'forward' | 'compensation';  // for color (green vs orange)
}

export interface SagaGraph {
  saga_id: string;
  description: string;
  nodes: SagaNode[];
  edges: SagaEdge[];
}

interface RawForwardStep {
  worker_id?: string;
  npc_id?: string;
  action?: string;
  payload?: { text?: string } | Record<string, unknown>;
}

interface RawCompensationStep {
  worker_id?: string;
  action?: string;
}

/** Parse Saga script YAML content into a graph.
 *  Throws Error if `saga_id` or `steps` missing or invalid.
 */
export function parseSagaToGraph(content: string): SagaGraph {
  let doc: unknown;
  try {
    doc = yaml.load(content);
  } catch (err) {
    throw new Error(
      `Invalid YAML: ${err instanceof Error ? err.message : String(err)}`,
    );
  }

  if (doc === null || typeof doc !== 'object') {
    throw new Error('Saga document must be a YAML mapping');
  }

  const raw = doc as Record<string, unknown>;

  const sagaId = raw.saga_id;
  if (typeof sagaId !== 'string' || sagaId.trim() === '') {
    throw new Error("Missing required field: 'saga_id'");
  }

  const description =
    typeof raw.description === 'string' ? raw.description : '';

  const stepsRaw = raw.steps;
  if (!Array.isArray(stepsRaw) || stepsRaw.length === 0) {
    throw new Error("Missing or empty required field: 'steps'");
  }

  const compRaw = raw.compensation;
  const hasCompensation = Array.isArray(compRaw) && compRaw.length > 0;
  const compensationSteps: RawCompensationStep[] = hasCompensation
    ? (compRaw as RawCompensationStep[])
    : [];

  const nodes: SagaNode[] = [];
  const edges: SagaEdge[] = [];

  // start node
  nodes.push({ id: 'start', label: 'start', type: 'start' });

  // forward nodes + chain
  const stepCount = stepsRaw.length;
  for (let i = 0; i < stepCount; i++) {
    const s = stepsRaw[i] as RawForwardStep;
    const stepId = `step_${i}`;
    const parts: string[] = [];
    if (s.worker_id) parts.push(s.worker_id);
    if (s.action) parts.push(s.action);
    const label = parts.length > 0 ? parts.join(' · ') : `step_${i}`;

    const text =
      s.payload && typeof s.payload === 'object' && 'text' in s.payload
        ? typeof s.payload.text === 'string'
          ? s.payload.text
          : undefined
        : undefined;

    nodes.push({
      id: stepId,
      label,
      type: 'forward',
      worker_id: s.worker_id,
      npc_id: s.npc_id,
      action: s.action,
      text,
    });
  }

  // forward edges: start -> step_0 -> ... -> step_(N-1) -> end
  const forwardChain: Array<{ source: string; target: string }> = [];
  forwardChain.push({ source: 'start', target: 'step_0' });
  for (let i = 0; i < stepCount - 1; i++) {
    forwardChain.push({ source: `step_${i}`, target: `step_${i + 1}` });
  }
  forwardChain.push({ source: `step_${stepCount - 1}`, target: 'end' });

  for (const fe of forwardChain) {
    edges.push({
      id: `e_${fe.source}_${fe.target}`,
      source: fe.source,
      target: fe.target,
      kind: 'forward',
    });
  }

  // compensation nodes + step[i] -> comp[i] edges
  if (hasCompensation) {
    const compCount = compensationSteps.length;
    for (let i = 0; i < compCount; i++) {
      const c = compensationSteps[i];
      const compId = `comp_${i}`;
      const parts: string[] = [];
      if (c.worker_id) parts.push(c.worker_id);
      if (c.action) parts.push(c.action);
      const label = parts.length > 0 ? parts.join(' · ') : `comp_${i}`;

      nodes.push({
        id: compId,
        label,
        type: 'compensation',
        worker_id: c.worker_id,
        action: c.action,
      });

      // pair comp[i] with step[i] (within bounds)
      if (i < stepCount) {
        edges.push({
          id: `e_step_${i}_comp_${i}`,
          source: `step_${i}`,
          target: compId,
          kind: 'compensation',
        });
      }
    }
  }

  // end node (push last so it sits at the tail of the chain visually)
  nodes.push({ id: 'end', label: 'end', type: 'end' });

  return {
    saga_id: sagaId,
    description,
    nodes,
    edges,
  };
}
