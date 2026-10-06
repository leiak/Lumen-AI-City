'use client';
import { Handle, Position, NodeProps } from 'reactflow';

const COLOR_FORWARD_BG = '#dcfce7';     // green-100
const COLOR_FORWARD_BORDER = '#16a34a';  // green-600
const COLOR_COMP_BG = '#fed7aa';          // orange-200
const COLOR_COMP_BORDER = '#ea580c';      // orange-600
const COLOR_TERMINAL_BG = '#e0e7ff';     // indigo-100
const COLOR_TERMINAL_BORDER = '#4f46e5'; // indigo-600

export interface SagaStepNodeData {
  label: string;
  type: 'forward' | 'compensation' | 'start' | 'end';
  worker_id?: string;
  npc_id?: string;
  action?: string;
  text?: string;
}

export default function SagaStepNode({ data }: NodeProps<SagaStepNodeData>) {
  const { type, label, worker_id, npc_id, action, text } = data;

  let bg = COLOR_FORWARD_BG, border = COLOR_FORWARD_BORDER;
  if (type === 'compensation') {
    bg = COLOR_COMP_BG; border = COLOR_COMP_BORDER;
  } else if (type === 'start' || type === 'end') {
    bg = COLOR_TERMINAL_BG; border = COLOR_TERMINAL_BORDER;
  }

  const isTerminal = type === 'start' || type === 'end';
  const showHandles = !isTerminal;
  // Single top + single bottom handle for vertical flow
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
      data-testid={`saga-node-${label}`}
    >
      {showHandles && <Handle type="target" position={Position.Top} style={{ background: border }} />}
      <div style={{ fontWeight: 600, marginBottom: 4 }}>
        {type === 'forward' ? '▶ ' : type === 'compensation' ? '◀ ' : ''}
        {label}
      </div>
      {worker_id && (
        <div style={{ fontSize: 11, color: '#4b5563' }}>worker: {worker_id}</div>
      )}
      {npc_id && (
        <div style={{ fontSize: 11, color: '#4b5563' }}>npc: {npc_id}</div>
      )}
      {action && (
        <div style={{ fontSize: 11, color: '#4b5563' }}>action: {action}</div>
      )}
      {text && (
        <div style={{ fontSize: 11, fontStyle: 'italic', marginTop: 4, color: '#374151' }}>
          &quot;{text}&quot;
        </div>
      )}
      {showHandles && <Handle type="source" position={Position.Bottom} style={{ background: border }} />}
    </div>
  );
}
