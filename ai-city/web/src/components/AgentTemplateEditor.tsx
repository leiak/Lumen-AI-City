'use client';

import { useCallback, useEffect, useState } from 'react';

const A2A_BASE = process.env.NEXT_PUBLIC_A2A_GATEWAY || 'http://localhost:8083';

interface EditorEdge {
  node_id: string;
  option_id: string;
  text: string;
  target: string;
}

interface EditorPayload {
  id: number;
  agent_id: string;
  name: string;
  status: string;
  version: number;
  talk_tree: Record<string, unknown>;
  bt_editor_tree: Record<string, unknown>;
  node_edges: EditorEdge[];
}

interface Revision {
  version: number;
  edited_by: string;
  created_at: string;
}

interface SimulationStep {
  node_id: string;
  say: string;
}

interface RunResult {
  status: string;
  steps: SimulationStep[];
  error?: string;
}

interface Props {
  agentId: string;
  templateId: number;
  templateName: string;
  onClosed: () => void;
  onSaved: () => void;
}

function requestMessage(error: unknown, fallback: string): string {
  if (error instanceof Error) return error.message;
  return fallback;
}

async function requestJson<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${A2A_BASE}${path}`, {
    ...init,
    headers: { 'Content-Type': 'application/json', ...init?.headers },
  });
  const data = await response.json().catch(() => ({}));
  if (!response.ok) {
    const message = data.message || data.error || data.detail?.msg || `HTTP ${response.status}`;
    throw new Error(String(message));
  }
  return data as T;
}

export function AgentTemplateEditor({
  agentId,
  templateId,
  templateName,
  onClosed,
  onSaved,
}: Props) {
  const [draft, setDraft] = useState<EditorPayload | null>(null);
  const [editorJson, setEditorJson] = useState('');
  const [revisions, setRevisions] = useState<Revision[]>([]);
  const [showRevisions, setShowRevisions] = useState(false);
  const [message, setMessage] = useState('');
  const [messageKind, setMessageKind] = useState<'ok' | 'error'>('ok');
  const [loading, setLoading] = useState(false);
  const [busyAction, setBusyAction] = useState<'validate' | 'save' | 'run' | null>(null);
  const [runResult, setRunResult] = useState<RunResult | null>(null);

  const notify = useCallback((kind: 'ok' | 'error', text: string) => {
    setMessageKind(kind);
    setMessage(text);
  }, []);

  const loadDraft = useCallback(async () => {
    setLoading(true);
    setRunResult(null);
    setShowRevisions(false);
    try {
      const data = await requestJson<EditorPayload>(
        `/v1/agent/actions/npc-templates/${templateId}/editor?agent_id=${encodeURIComponent(agentId)}`,
      );
      setDraft(data);
      setEditorJson(JSON.stringify(data.bt_editor_tree, null, 2));
      notify('ok', `Draft #${templateId} loaded at version ${data.version}`);
    } catch (error) {
      notify('error', requestMessage(error, 'Draft load failed'));
    } finally {
      setLoading(false);
    }
  }, [agentId, notify, templateId]);

  useEffect(() => {
    void loadDraft();
  }, [loadDraft]);

  const parsedEditorTree = useCallback(() => {
    const value = JSON.parse(editorJson);
    if (!value || typeof value !== 'object' || Array.isArray(value)) {
      throw new Error('BT tree must be a JSON object');
    }
    return value as Record<string, unknown>;
  }, [editorJson]);

  const validate = useCallback(async () => {
    setBusyAction('validate');
    try {
      const result = await requestJson<{ valid: boolean; source: string; validation_trace: Record<string, unknown> }>(
        `/v1/agent/actions/npc-templates/${templateId}/editor/validate`,
        { method: 'POST', body: JSON.stringify({ agent_id: agentId, bt_editor_tree: parsedEditorTree() }) },
      );
      notify('ok', `Valid ${result.source}: ${String(result.validation_trace.nodes ?? '')} nodes`);
    } catch (error) {
      notify('error', requestMessage(error, 'Validation failed'));
    } finally {
      setBusyAction(null);
    }
  }, [agentId, notify, parsedEditorTree, templateId]);

  const save = useCallback(async () => {
    setBusyAction('save');
    try {
      const saved = await requestJson<EditorPayload>(
        `/v1/agent/actions/npc-templates/${templateId}/editor`,
        { method: 'PUT', body: JSON.stringify({ agent_id: agentId, bt_editor_tree: parsedEditorTree() }) },
      );
      setDraft(saved);
      setEditorJson(JSON.stringify(saved.bt_editor_tree, null, 2));
      onSaved();
      notify('ok', `Saved version ${saved.version}`);
    } catch (error) {
      notify('error', requestMessage(error, 'Save failed'));
    } finally {
      setBusyAction(null);
    }
  }, [agentId, notify, onSaved, parsedEditorTree, templateId]);

  const run = useCallback(async () => {
    setBusyAction('run');
    try {
      const result = await requestJson<RunResult>(
        `/v1/agent/actions/npc-templates/${templateId}/editor/run`,
        { method: 'POST', body: JSON.stringify({ agent_id: agentId, bt_editor_tree: parsedEditorTree(), choices: [] }) },
      );
      setRunResult(result);
      notify('ok', `Sandbox: ${result.status}`);
    } catch (error) {
      notify('error', requestMessage(error, 'Sandbox failed'));
    } finally {
      setBusyAction(null);
    }
  }, [agentId, notify, parsedEditorTree, templateId]);

  const loadRevisions = useCallback(async () => {
    const nextVisible = !showRevisions;
    setShowRevisions(nextVisible);
    if (!nextVisible) return;
    try {
      const data = await requestJson<{ revisions: Revision[] }>(
        `/v1/agent/actions/npc-templates/${templateId}/editor/revisions?agent_id=${encodeURIComponent(agentId)}`,
      );
      setRevisions(data.revisions);
    } catch (error) {
      notify('error', requestMessage(error, 'Revision load failed'));
    }
  }, [agentId, notify, showRevisions, templateId]);

  const rollback = useCallback(async (version: number) => {
    try {
      await requestJson<EditorPayload>(
        `/v1/agent/actions/npc-templates/${templateId}/editor/rollback`,
        { method: 'POST', body: JSON.stringify({ agent_id: agentId, version }) },
      );
      await loadDraft();
      onSaved();
      notify('ok', `Rolled back to version ${version}`);
    } catch (error) {
      notify('error', requestMessage(error, 'Rollback failed'));
    }
  }, [agentId, loadDraft, notify, onSaved, templateId]);

  const editable = draft?.status === 'draft' || draft?.status === 'rejected';

  return (
    <div className="rounded-xl border border-purple-500/30 bg-gray-950/80 p-3" data-testid="agent-template-editor">
      <div className="flex items-center justify-between">
        <div>
          <div className="text-xs font-semibold text-white">BT Editor · #{templateId}</div>
          <div className="text-[10px] text-gray-500">{templateName}</div>
        </div>
        <div className="flex gap-1">
          <button onClick={() => void loadDraft()} disabled={loading} className="rounded bg-gray-800 px-2 py-1 text-[10px] text-gray-200 hover:bg-gray-700">Reload</button>
          <button onClick={onClosed} className="rounded bg-gray-800 px-2 py-1 text-[10px] text-gray-200 hover:bg-gray-700">Close</button>
        </div>
      </div>

      <textarea
        value={editorJson}
        onChange={(event) => setEditorJson(event.target.value)}
        disabled={loading || !editable}
        spellCheck={false}
        className="mt-2 h-44 w-full rounded-lg bg-gray-900 border border-white/10 p-2 font-mono text-[11px] text-gray-100 outline-none disabled:opacity-50"
        data-testid="agent-template-editor-json"
      />

      <div className="mt-2 grid grid-cols-2 gap-1">
        <button onClick={() => void validate()} disabled={!editable || busyAction !== null} className="rounded bg-blue-600 px-2 py-1 text-[10px] text-white hover:bg-blue-500 disabled:opacity-40">{busyAction === 'validate' ? 'Validating...' : 'Validate'}</button>
        <button onClick={() => void save()} disabled={!editable || busyAction !== null} className="rounded bg-emerald-600 px-2 py-1 text-[10px] text-white hover:bg-emerald-500 disabled:opacity-40">{busyAction === 'save' ? 'Saving...' : 'Save version'}</button>
        <button onClick={() => void run()} disabled={busyAction !== null} className="rounded bg-purple-600 px-2 py-1 text-[10px] text-white hover:bg-purple-500 disabled:opacity-40">{busyAction === 'run' ? 'Running...' : 'Sandbox run'}</button>
        <button onClick={() => void loadRevisions()} className="rounded bg-gray-700 px-2 py-1 text-[10px] text-white hover:bg-gray-600">{showRevisions ? 'Hide versions' : 'Versions'}</button>
      </div>

      {message && (
        <div className={`mt-2 text-[10px] ${messageKind === 'ok' ? 'text-emerald-400' : 'text-red-400'}`}>{message}</div>
      )}

      {runResult && (
        <div className="mt-2 rounded-lg bg-gray-900/80 p-2 text-[10px] text-gray-400">
          <div className="text-gray-300">Trace: {runResult.status}</div>
          {runResult.steps.map((step, index) => (
            <div key={`${step.node_id}-${index}`}>{index + 1}. {step.node_id}{step.say ? ` · ${step.say}` : ''}</div>
          ))}
          {runResult.error && <div className="text-red-400">{runResult.error}</div>}
        </div>
      )}

      {showRevisions && (
        <div className="mt-2 space-y-1">
          {revisions.length === 0 && <div className="text-[10px] text-gray-500">No saved versions.</div>}
          {revisions.map((revision) => (
            <div key={revision.version} className="flex items-center justify-between rounded bg-gray-900/80 px-2 py-1 text-[10px] text-gray-400">
              <span>v{revision.version}</span>
              <button onClick={() => void rollback(revision.version)} disabled={!editable} className="rounded bg-gray-800 px-2 py-0.5 text-gray-200 hover:bg-gray-700 disabled:opacity-40">Rollback</button>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
