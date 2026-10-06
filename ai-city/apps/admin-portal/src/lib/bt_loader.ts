/** BT (Behavior Tree) loader — Phase C.3
 *
 *  Recursively walks a BT JSON document and converts it into a flat
 *  React-Flow-friendly graph (nodes + edges). All node types are normalised
 *  to lowercase to match the editor UI convention; persisted JSON may
 *  contain PascalCase forms (Sequence / Condition / ...) per
 *  `packages/bt-editor-spec/bt-node.schema.json`.
 *
 *  7 canonical BT node types:
 *    sequence | selector | action | condition | decorator | subtree | llm
 */

export const BT_NODE_TYPES = [
  'sequence',
  'selector',
  'action',
  'condition',
  'decorator',
  'subtree',
  'llm',
] as const;

export type BTNodeType = (typeof BT_NODE_TYPES)[number];

/** Raw shape accepted from the editor / persisted JSON. PascalCase or lowercase. */
export interface BTJSONNode {
  id: string;
  type: string;
  name?: string;
  metadata?: Record<string, unknown>;
  // sequence / selector
  children?: BTJSONNode[];
  // condition
  expression?: string;
  true_action?: string;
  false_action?: string;
  // action
  action?: string;
  args?: Record<string, unknown>;
  // decorator
  decorator?: string;
  child?: BTJSONNode;
  // subtree
  ref?: string;
  // llm
  prompt_template?: string;
  model?: string;
  max_tokens?: number;
  tools?: string[];
}

export interface BTJSON {
  version: string;
  name?: string;
  description?: string;
  author?: string;
  tags?: string[];
  root: BTJSONNode;
  subtrees?: Record<string, BTJSONNode>;
  metadata?: Record<string, unknown>;
  created_at_ms?: number;
  updated_at_ms?: number;
}

/** Flattened graph node consumed by React Flow / dagre. */
export interface BTNode {
  id: string;
  type: BTNodeType;
  label: string;
  // action
  action?: string;
  args?: Record<string, unknown>;
  // condition
  expression?: string;
  true_action?: string;
  false_action?: string;
  // decorator
  decorator?: string;
  // subtree
  ref?: string;
  // llm
  prompt_template?: string;
  model?: string;
  max_tokens?: number;
  tools?: string[];
  // misc
  name?: string;
  metadata?: Record<string, unknown>;
}

export interface BTEdge {
  id: string;
  source: string;
  target: string;
}

export interface BTGraph {
  version: string;
  description?: string;
  rootId: string;
  nodes: BTNode[];
  edges: BTEdge[];
}

const TYPE_LOOKUP: Record<string, BTNodeType> = {
  sequence: 'sequence',
  selector: 'selector',
  action: 'action',
  condition: 'condition',
  decorator: 'decorator',
  subtree: 'subtree',
  llm: 'llm',
};

/** Normalise a string type into one of the 7 canonical BT types. */
function normalizeType(raw: unknown): BTNodeType {
  if (typeof raw !== 'string') {
    throw new Error("BT node missing required 'type'");
  }
  const key = raw.toLowerCase();
  const t = TYPE_LOOKUP[key];
  if (!t) {
    throw new Error(
      `Unknown BT node type: '${raw}' (expected one of ${BT_NODE_TYPES.join(', ')})`,
    );
  }
  return t;
}

/** Build a short display label for a node. */
function buildLabel(n: BTJSONNode): string {
  if (n.name && n.name.trim().length > 0) return n.name;
  switch (n.type.toLowerCase()) {
    case 'action':
      return n.action ? `action: ${n.action}` : 'action';
    case 'condition':
      return n.expression ? `cond: ${truncate(n.expression, 20)}` : 'condition';
    case 'decorator':
      return n.decorator ? `decorator: ${n.decorator}` : 'decorator';
    case 'subtree':
      return n.ref ? `subtree: ${n.ref}` : 'subtree';
    case 'llm':
      return 'llm';
    case 'sequence':
      return 'sequence';
    case 'selector':
      return 'selector';
    default:
      return n.id;
  }
}

function truncate(s: string, max: number): string {
  return s.length > max ? `${s.slice(0, max - 1)}…` : s;
}

/** Convert one raw JSON node into the flat BTNode shape (no recursion). */
function toBTNode(n: BTJSONNode): BTNode {
  const type = normalizeType(n.type);
  const base: BTNode = {
    id: n.id,
    type,
    label: buildLabel(n),
  };
  // copy optional fields based on type
  if (n.name) base.name = n.name;
  if (n.metadata) base.metadata = n.metadata;

  if (type === 'action') {
    if (n.action) base.action = n.action;
    if (n.args) base.args = n.args;
  } else if (type === 'condition') {
    if (n.expression) base.expression = n.expression;
    if (n.true_action) base.true_action = n.true_action;
    if (n.false_action) base.false_action = n.false_action;
  } else if (type === 'decorator') {
    if (n.decorator) base.decorator = n.decorator;
  } else if (type === 'subtree') {
    if (n.ref) base.ref = n.ref;
  } else if (type === 'llm') {
    if (n.prompt_template) base.prompt_template = n.prompt_template;
    if (n.model) base.model = n.model;
    if (typeof n.max_tokens === 'number') base.max_tokens = n.max_tokens;
    if (n.tools) base.tools = n.tools;
  }
  return base;
}

let edgeCounter = 0;
function makeEdgeId(src: string, tgt: string): string {
  edgeCounter += 1;
  return `e_${src}_${tgt}_${edgeCounter}`;
}

/** Parse a BT JSON document into a React Flow graph.
 *  Throws on missing root.id, root.type, or unknown type.
 */
export function parseBtTreeToGraph(json: BTJSON): BTGraph {
  edgeCounter = 0;

  if (!json || typeof json !== 'object' || !json.root) {
    throw new Error("BT document missing required field: 'root'");
  }
  const root = json.root;
  if (typeof root.id !== 'string' || root.id.length === 0) {
    throw new Error("BT root missing required field: 'id'");
  }

  const nodes: BTNode[] = [];
  const edges: BTEdge[] = [];
  const seen = new Set<string>();
  const subtreeIndex: Record<string, BTJSONNode> = { ...(json.subtrees ?? {}) };

  function walk(n: BTJSONNode, parentId: string | null): void {
    if (typeof n.id !== 'string' || n.id.length === 0) {
      throw new Error("BT node missing required field: 'id'");
    }
    // duplicate-id detection (also catches cycles since we'd re-emit)
    if (seen.has(n.id)) {
      throw new Error(`Duplicate BT node id: '${n.id}'`);
    }
    seen.add(n.id);

    nodes.push(toBTNode(n));

    if (parentId) {
      edges.push({
        id: makeEdgeId(parentId, n.id),
        source: parentId,
        target: n.id,
      });
    }

    const normType = n.type.toLowerCase();
    if (normType === 'sequence' || normType === 'selector') {
      const children = Array.isArray(n.children) ? n.children : [];
      for (const c of children) walk(c, n.id);
    } else if (normType === 'decorator') {
      if (n.child) walk(n.child, n.id);
    } else if (normType === 'subtree') {
      // Inline subtree body if available in subtrees map (ref → body)
      const ref = n.ref;
      if (ref && subtreeIndex[ref]) {
        const body = subtreeIndex[ref];
        // rename inline body ids to avoid collision
        const inlinedId = `${n.id}__body`;
        walk({ ...body, id: inlinedId }, n.id);
      }
    }
    // action / condition / llm: no children
  }

  walk(root, null);

  return {
    version: json.version,
    description: json.description,
    rootId: root.id,
    nodes,
    edges,
  };
}
