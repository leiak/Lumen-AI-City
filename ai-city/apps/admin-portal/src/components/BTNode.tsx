'use client';
import { Handle, Position, NodeProps } from 'reactflow';
import type { BTNodeType } from '@/lib/bt_loader';

const COLORS: Record<BTNodeType, { bg: string; border: string }> = {
  sequence: { bg: '#e0e7ff', border: '#4f46e5' },     // indigo
  selector: { bg: '#f3e8ff', border: '#9333ea' },     // purple
  action: { bg: '#dcfce7', border: '#16a34a' },       // green
  condition: { bg: '#ffedd5', border: '#ea580c' },    // orange
  decorator: { bg: '#fce7f3', border: '#db2777' },    // pink
  subtree: { bg: '#ccfbf1', border: '#0d9488' },      // teal
  llm: { bg: '#fef9c3', border: '#a16207' },          // yellow
};

/** Payload passed to the React Flow <BTNode> via `data`. Mirrors the
 *  flat BTNode shape produced by parseBtTreeToGraph() so the renderer
 *  doesn't have to dig through nested raw objects. */
export interface BTNodeData {
  label: string;
  type: BTNodeType;
  node: {
    id: string;
    type: BTNodeType;
    name?: string;
    action?: string;
    args?: Record<string, unknown>;
    expression?: string;
    decorator?: string;
    ref?: string;
    prompt_template?: string;
    model?: string;
    max_tokens?: number;
  };
}

export default function BTNode({ data }: NodeProps<BTNodeData>) {
  const { type: nodeType, label, node } = data;
  const { bg, border } = COLORS[nodeType];
  const isComposite = nodeType === 'sequence' || nodeType === 'selector';

  return (
    <div
      style={{
        background: bg,
        border: `2px solid ${border}`,
        borderRadius: 8,
        padding: 10,
        minWidth: 160,
        maxWidth: 240,
        fontSize: 13,
        color: '#1f2937',
        boxShadow: '0 1px 3px rgba(0,0,0,0.08)',
      }}
      data-testid={`bt-node-${node.id}`}
    >
      {isComposite && (
        <Handle type="target" position={Position.Top} style={{ background: border }} />
      )}
      <div style={{ fontWeight: 600, marginBottom: 4 }}>{label}</div>
      <div style={{ fontSize: 11, color: '#4b5563' }}>type: {nodeType}</div>

      {nodeType === 'action' && node.action && (
        <div style={{ fontSize: 11, color: '#4b5563' }}>action: {node.action}</div>
      )}
      {nodeType === 'action' && node.args && Object.keys(node.args).length > 0 && (
        <div style={{ fontSize: 11, color: '#4b5563' }}>
          args: {JSON.stringify(node.args)}
        </div>
      )}
      {nodeType === 'condition' && node.expression && (
        <div style={{ fontSize: 11, fontStyle: 'italic', color: '#374151' }}>
          {node.expression}
        </div>
      )}
      {nodeType === 'decorator' && node.decorator && (
        <div style={{ fontSize: 11, color: '#4b5563' }}>decorator: {node.decorator}</div>
      )}
      {nodeType === 'subtree' && node.ref && (
        <div style={{ fontSize: 11, color: '#4b5563' }}>ref: {node.ref}</div>
      )}
      {nodeType === 'llm' && node.prompt_template && (
        <div style={{ fontSize: 11, fontStyle: 'italic', marginTop: 4, color: '#374151' }}>
          &quot;{node.prompt_template}&quot;
        </div>
      )}
      {nodeType === 'llm' && node.model && (
        <div style={{ fontSize: 11, color: '#4b5563' }}>model: {node.model}</div>
      )}

      {isComposite && (
        <Handle type="source" position={Position.Bottom} style={{ background: border }} />
      )}
    </div>
  );
}