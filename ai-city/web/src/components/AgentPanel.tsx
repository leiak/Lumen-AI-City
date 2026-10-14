'use client';

import { useState, useCallback, useEffect, useRef } from 'react';

import { AgentTemplateEditor } from './AgentTemplateEditor';

const A2A_BASE = process.env.NEXT_PUBLIC_A2A_GATEWAY || 'http://localhost:8083';

interface AgentCard {
  agent_id: string;
  name: string;
  provider: string;
  capabilities: string[];
  city_id: string;
}

interface BehaviorOption {
  id: string;
  text: string;
}

interface BehaviorNode {
  say: string;
  options: BehaviorOption[];
}

interface BehaviorTree {
  npc_id: string;
  name: string;
  initial: string;
  default_say: string;
  nodes: Record<string, BehaviorNode>;
}

interface InboxMessage {
  message_id: string;
  from_agent_id: string;
  to_agent_id: string;
  type: string;
  payload: string;
  trace_id?: string;
}

interface ChatEntry {
  role: 'agent' | 'npc' | 'system';
  from: string;
  text: string;
  contact: string;
  messageId?: string;
  traceId?: string;
  ts: number;
}

interface TemplateDraft {
  id: number;
  name: string;
  status: string;
  validation_trace?: Record<string, unknown>;
  simulation_trace?: Record<string, unknown>;
  reviewed_by?: string;
  review_reason?: string;
}

function inboxCursorKey(agentId: string): string {
  return `aicity_agent_inbox_cursor_${agentId}`;
}

export function AgentPanel() {
  const [agentId, setAgentId] = useState('my-bot-' + Date.now().toString(36));
  const [registered, setRegistered] = useState(false);
  const [gatewayOnline, setGatewayOnline] = useState<boolean | null>(null);
  const [status, setStatus] = useState('');
  const [cards, setCards] = useState<AgentCard[]>([]);
  const [selectedContact, setSelectedContact] = useState('');
  const [selectedNpc, setSelectedNpc] = useState('');
  const [tab, setTab] = useState<'chat' | 'world' | 'behavior' | 'template'>('chat');
  const [behavior, setBehavior] = useState<BehaviorTree | null>(null);
  const [behaviorNode, setBehaviorNode] = useState('');
  const [tileId, setTileId] = useState('tile_1_0');
  const [busy, setBusy] = useState(false);
  const [messages, setMessages] = useState<ChatEntry[]>([]);
  const [input, setInput] = useState('');
  const [sending, setSending] = useState(false);
  const [templateName, setTemplateName] = useState('');
  const [templateTile, setTemplateTile] = useState('tile_1_0');
  const [templateSay, setTemplateSay] = useState('Hello, traveler!');
  const [templateDrafts, setTemplateDrafts] = useState<TemplateDraft[]>([]);
  const [templateMessage, setTemplateMessage] = useState('');
  const [creatingTemplate, setCreatingTemplate] = useState(false);
  const [submittingTemplate, setSubmittingTemplate] = useState<number | null>(null);
  const [simulatingTemplate, setSimulatingTemplate] = useState<number | null>(null);
  const [editingTemplateId, setEditingTemplateId] = useState<number | null>(null);
  const inboxTimer = useRef<ReturnType<typeof setInterval> | null>(null);
  const inboxCursor = useRef<string>('');
  const chatRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (chatRef.current) chatRef.current.scrollTop = chatRef.current.scrollHeight;
  }, [messages]);

  useEffect(() => {
    let cancelled = false;
    const checkGateway = async () => {
      try {
        const resp = await fetch(`${A2A_BASE}/v1/healthz`);
        if (!cancelled) setGatewayOnline(resp.ok);
      } catch {
        if (!cancelled) setGatewayOnline(false);
      }
    };
    checkGateway();
    return () => {
      cancelled = true;
    };
  }, []);

  const addMsg = useCallback((
    role: ChatEntry['role'],
    from: string,
    text: string,
    contact = 'system',
    metadata?: { messageId?: string; traceId?: string },
  ) => {
    setMessages((m) => [...m, {
      role,
      from,
      text,
      contact,
      messageId: metadata?.messageId,
      traceId: metadata?.traceId,
      ts: Date.now(),
    }]);
  }, []);

  const register = useCallback(async () => {
    setBusy(true);
    setStatus('Registering...');
    try {
      const resp = await fetch(`${A2A_BASE}/v1/cards`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          agent_id: agentId,
          name: agentId,
          provider: 'openclaw',
          capabilities: ['dialogue'],
          city_id: 'city_a',
        }),
      });
      const data = await resp.json();
      if (data.accepted) {
        setRegistered(true);
        inboxCursor.current = window.localStorage.getItem(inboxCursorKey(agentId)) || '';
        setStatus(`Registered as ${agentId}`);
        addMsg('system', 'gateway', `Agent ${agentId} registered`);
      } else {
        setStatus(data.message || 'Registration failed');
        addMsg('system', 'gateway', data.message || 'Registration failed');
      }
    } catch (e) {
      setStatus(`Gateway error: ${e}`);
      addMsg('system', 'gateway', `Error: ${e}`);
    } finally {
      setBusy(false);
    }
  }, [agentId, addMsg]);

  const discover = useCallback(async () => {
    try {
      const resp = await fetch(`${A2A_BASE}/v1/discover?capability=dialogue&city_filter=city_a`);
      const data = await resp.json();
      const contacts = data.cards || [];
      const npcs = contacts.filter((c: AgentCard) => c.agent_id.startsWith('npc_'));
      setCards(contacts);
      if (npcs.length > 0 && !selectedNpc) setSelectedNpc(npcs[0].agent_id);
      if (!selectedContact) setSelectedContact(npcs[0]?.agent_id || contacts[0]?.agent_id || '');
      addMsg('system', 'gateway', `Found ${contacts.length} contacts (${npcs.length} NPCs)`);
    } catch (e) {
      addMsg('system', 'gateway', `Discover error: ${e}`);
    }
  }, [selectedContact, selectedNpc, addMsg]);

  const send = useCallback(async () => {
    if (!input.trim() || !registered || !selectedContact) return;
    setSending(true);
    const messageId = `msg-${Date.now()}`;
    const traceId = `trace-${Date.now().toString(36)}`;
    addMsg('agent', agentId, input.trim(), selectedContact, { messageId, traceId });
    try {
      await fetch(`${A2A_BASE}/v1/messages`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          message_id: messageId,
          from_agent_id: agentId,
          to_agent_id: selectedContact,
          type: 'request',
          payload: JSON.stringify({ text: input.trim() }),
          trace_id: traceId,
        }),
      });
    } catch (e) {
      addMsg('system', 'gateway', `Send error: ${e}`);
    }
    setInput('');
    setSending(false);
  }, [input, agentId, selectedContact, registered, addMsg]);

  const move = useCallback(async (targetTile: string) => {
    if (!registered || !targetTile.trim()) return;
    setBusy(true);
    try {
      const resp = await fetch(`${A2A_BASE}/v1/agent/actions/move`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ agent_id: agentId, tile_id: targetTile.trim() }),
      });
      const data = await resp.json();
      if (resp.ok) {
        addMsg('system', 'world', `${agentId} → ${data.current_tile_id}`);
      } else {
        addMsg('system', 'world', data.error?.detail || data.message || 'Move rejected');
      }
    } catch (e) {
      addMsg('system', 'world', `Move error: ${e}`);
    } finally {
      setBusy(false);
    }
  }, [agentId, registered, addMsg]);

  const loadBehavior = useCallback(async (npcID = selectedNpc) => {
    if (!npcID) return;
    setBusy(true);
    try {
      const resp = await fetch(`${A2A_BASE}/v1/agent/actions/npc-behavior?npc_id=${encodeURIComponent(npcID)}`);
      const data = await resp.json();
      if (!resp.ok) throw new Error(data.message || 'Not found');
      setBehavior(data);
      setBehaviorNode(data.initial);
    } catch (e) {
      addMsg('system', 'behavior', `Load failed: ${e}`);
      setBehavior(null);
    } finally {
      setBusy(false);
    }
  }, [selectedNpc, addMsg]);

  const runBehaviorNode = useCallback(async (nodeID: string) => {
    if (!behavior || !nodeID) return;
    setBusy(true);
    try {
      const resp = await fetch(`${A2A_BASE}/v1/agent/actions/npc-talk`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ agent_id: agentId, npc_id: behavior.npc_id, node_id: nodeID }),
      });
      const data = await resp.json();
      if (!resp.ok) throw new Error(data.message || 'Node rejected');
      setBehaviorNode(nodeID);
      addMsg('npc', data.npc_id, data.say, behavior.npc_id);
    } catch (e) {
      addMsg('system', 'behavior', `Action failed: ${e}`);
    } finally {
      setBusy(false);
    }
  }, [agentId, behavior, addMsg]);

  const startInbox = useCallback(() => {
    if (inboxTimer.current) clearInterval(inboxTimer.current);
    inboxTimer.current = setInterval(async () => {
      if (!registered) return;
      try {
        const params = new URLSearchParams({
          limit: '20',
          mark_read: 'true',
        });
        if (inboxCursor.current) params.set('cursor', inboxCursor.current);
        const resp = await fetch(`${A2A_BASE}/v1/inbox/${agentId}?${params.toString()}`);
        const data = await resp.json();
        if (data.next_cursor) {
          inboxCursor.current = data.next_cursor;
          window.localStorage.setItem(inboxCursorKey(agentId), data.next_cursor);
        }
        for (const msg of data.messages || []) {
          let text = msg.payload;
          try {
            const p = JSON.parse(msg.payload);
            text = p.say || p.text || msg.payload;
          } catch { /* raw */ }
          addMsg('npc', msg.from_agent_id, text, msg.from_agent_id, {
            messageId: msg.message_id,
            traceId: msg.trace_id,
          });
        }
      } catch { /* silent */ }
    }, 2000);
  }, [agentId, registered, addMsg]);

  const loadTemplateDrafts = useCallback(async () => {
    try {
      const resp = await fetch(`${A2A_BASE}/v1/agent/actions/npc-templates?agent_id=${encodeURIComponent(agentId)}`);
      if (!resp.ok) throw new Error('load drafts rejected');
      const data = await resp.json();
      setTemplateDrafts(data.drafts || []);
    } catch (e) {
      setTemplateMessage(`Draft load failed: ${e}`);
    }
  }, [agentId]);

  const createTemplateDraft = useCallback(async () => {
    if (!templateName.trim()) return;
    setCreatingTemplate(true);
    setTemplateMessage('Creating draft...');
    try {
      const resp = await fetch(`${A2A_BASE}/v1/agent/actions/npc-templates`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          agent_id: agentId,
          name: templateName.trim(),
          home_tile_id: templateTile.trim(),
          description: 'Created by Agent Console',
          personality: { O: 0.6, C: 0.7, E: 0.6, A: 0.7, N: 0.3 },
          talk_tree: {
            initial: 'root',
            nodes: { root: { say: templateSay || 'Hello, traveler!', options: [] } },
          },
          product_catalog: [],
        }),
      });
      const data = await resp.json();
      if (!resp.ok) {
        const message = data.detail?.msg || data.error || `HTTP ${resp.status}`;
        throw new Error(message);
      }
      setTemplateMessage(`Draft #${data.id} created`);
      setTemplateName('');
      await loadTemplateDrafts();
    } catch (e) {
      setTemplateMessage(`Create failed: ${e}`);
    } finally {
      setCreatingTemplate(false);
    }
  }, [agentId, templateName, templateTile, templateSay, loadTemplateDrafts]);

  const submitTemplateDraft = useCallback(async (id: number) => {
    setSubmittingTemplate(id);
    try {
      const resp = await fetch(`${A2A_BASE}/v1/agent/actions/npc-templates/${id}/submit`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ agent_id: agentId }),
      });
      const data = await resp.json();
      if (!resp.ok) throw new Error(data.detail?.msg || `HTTP ${resp.status}`);
      setTemplateMessage(`Draft #${id} submitted for review`);
      await loadTemplateDrafts();
    } catch (e) {
      setTemplateMessage(`Submit failed: ${e}`);
    } finally {
      setSubmittingTemplate(null);
    }
  }, [agentId, loadTemplateDrafts]);

  const simulateTemplateDraft = useCallback(async (id: number) => {
    setSimulatingTemplate(id);
    try {
      const resp = await fetch(`${A2A_BASE}/v1/agent/actions/npc-templates/${id}/simulate`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ agent_id: agentId }),
      });
      const data = await resp.json();
      if (!resp.ok) throw new Error(data.detail?.msg || `HTTP ${resp.status}`);
      setTemplateMessage(`Sandbox #${id}: ${data.status}${data.error ? ` (${data.error})` : ''}`);
      await loadTemplateDrafts();
    } catch (e) {
      setTemplateMessage(`Simulation failed: ${e}`);
    } finally {
      setSimulatingTemplate(null);
    }
  }, [agentId, loadTemplateDrafts]);

  useEffect(() => {
    if (registered) startInbox();
    return () => { if (inboxTimer.current) clearInterval(inboxTimer.current); };
  }, [registered, startInbox]);

  useEffect(() => {
    if (registered && tab === 'template') loadTemplateDrafts();
  }, [registered, tab, loadTemplateDrafts]);

  useEffect(() => { discover(); }, []);

  const visibleMessages = messages.filter(
    (m) => m.contact === selectedContact || m.contact === 'system'
  );

  return (
    <div className="absolute top-4 left-4 right-4 sm:left-auto sm:w-80 bg-gray-900/90 backdrop-blur-xl border border-white/10 rounded-2xl shadow-2xl overflow-hidden flex flex-col max-h-[calc(100vh-32px)]">
      <div className="px-4 py-3 bg-gradient-to-r from-indigo-600/40 to-purple-600/40 border-b border-white/10">
        <div className="flex items-center gap-2">
          <span className={`w-2 h-2 rounded-full ${
            gatewayOnline === null ? 'bg-yellow-400' : gatewayOnline ? 'bg-green-400 animate-pulse' : 'bg-red-500'
          }`} />
          <span className="font-semibold text-sm text-white">Agent Console</span>
          {registered && <span className="ml-auto text-xs text-green-400">● online</span>}
        </div>
        {gatewayOnline === false && (
          <div className="mt-2 rounded-lg bg-red-500/10 px-2 py-1 text-xs text-red-300">
            A2A gateway unreachable at {A2A_BASE}
          </div>
        )}
        {registered && (
          <div className="mt-2 grid grid-cols-4 gap-1 text-xs">
            {(['chat', 'world', 'behavior', 'template'] as const).map((item) => (
              <button key={item} onClick={() => setTab(item)}
                className={`rounded-md py-1 capitalize transition ${tab === item ? 'bg-indigo-600 text-white' : 'bg-gray-800/80 text-gray-300 hover:bg-gray-700'}`}>
                {item === 'behavior' ? 'Tree' : item}
              </button>
            ))}
          </div>
        )}
      </div>

      {!registered ? (
        <div className="p-4 space-y-3">
          <label className="text-xs text-gray-400">Agent ID</label>
          <input value={agentId} onChange={(e) => setAgentId(e.target.value)}
            className="w-full bg-gray-800/80 border border-white/10 rounded-lg px-3 py-2 text-sm text-white placeholder-gray-500 focus:border-indigo-500 outline-none" />
          <button onClick={register}
            disabled={busy || gatewayOnline === false}
            className="w-full py-2 rounded-lg bg-indigo-600 hover:bg-indigo-500 text-white text-sm font-medium transition">
            {busy ? 'Registering...' : 'Register Agent'}
          </button>
          {status && <div className="text-xs text-gray-400">{status}</div>}
        </div>
      ) : (
        <>
          {tab === 'chat' && (
            <>
              <div className="px-4 pt-3 pb-2 space-y-2">
                <div className="max-h-24 overflow-y-auto rounded-lg bg-gray-800/60 p-2">
                  <div className="text-[10px] uppercase tracking-wider text-gray-500 mb-1">A2A contacts</div>
                  <div className="flex flex-wrap gap-1">
                    {cards.map((c) => (
                      <button key={c.agent_id} onClick={() => setSelectedContact(c.agent_id)}
                        className={`rounded-full px-2 py-0.5 text-[10px] transition ${
                          selectedContact === c.agent_id ? 'bg-indigo-600 text-white' : 'bg-gray-700/80 text-gray-200 hover:bg-gray-600'
                        }`}>
                        {c.agent_id.startsWith('npc_') ? 'NPC ' : 'Bot '}
                        {c.name || c.agent_id}
                      </button>
                    ))}
                  </div>
                </div>
                <button onClick={discover} className="w-full py-1 rounded-lg bg-gray-800 hover:bg-gray-700 text-gray-300 text-xs">Refresh registry</button>
              </div>
              <div ref={chatRef} className="flex-1 overflow-y-auto px-4 pb-2 space-y-2 min-h-0" style={{ maxHeight: '42vh' }}>
                {messages.length === 0 && (
                  <div className="text-gray-500 text-xs text-center py-8">Select a contact and send an A2A message</div>
                )}
                {messages.map((m, i) => (
                  <div key={i} className={`text-xs ${m.role === 'agent' ? 'text-right' : 'text-left'}`}>
                    <span className={`font-medium ${m.role === 'agent' ? 'text-indigo-400' : m.role === 'npc' ? 'text-emerald-400' : 'text-gray-500'}`}>
                      {m.from}
                    </span>
                    <div className={`inline-block rounded-lg px-2.5 py-1.5 mt-0.5 max-w-[85%] ${
                      m.role === 'agent' ? 'bg-indigo-600/80 text-white rounded-br-none' :
                      m.role === 'npc' ? 'bg-gray-700/80 text-gray-100 rounded-bl-none' :
                      'bg-transparent text-gray-500'
                    }`}>{m.text}</div>
                  </div>
                ))}
              </div>
              <div className="flex border-t border-white/10">
                <input value={input} onChange={(e) => setInput(e.target.value)}
                  onKeyDown={(e) => e.key === 'Enter' && send()}
                  className="flex-1 bg-transparent px-3 py-2.5 text-sm text-white placeholder-gray-500 outline-none"
                  placeholder={selectedContact ? `Message ${selectedContact}...` : 'Select contact first'} />
                <button onClick={send} disabled={sending || !input.trim()}
                  className="px-4 bg-indigo-600 hover:bg-indigo-500 disabled:opacity-40 text-white text-sm transition">➤</button>
              </div>
            </>
          )}

          {tab === 'template' && (
            <div className="space-y-2 p-3">
              <div className="text-[10px] uppercase tracking-wider text-gray-500">NPC draft</div>
              <input value={templateName} onChange={(e) => setTemplateName(e.target.value)}
                placeholder="NPC name"
                className="w-full bg-gray-800/80 border border-white/10 rounded-lg px-2 py-1.5 text-xs text-white outline-none" />
              <input value={templateTile} onChange={(e) => setTemplateTile(e.target.value)}
                placeholder="Home tile"
                className="w-full bg-gray-800/80 border border-white/10 rounded-lg px-2 py-1.5 text-xs text-white outline-none" />
              <input value={templateSay} onChange={(e) => setTemplateSay(e.target.value)}
                placeholder="Opening line"
                className="w-full bg-gray-800/80 border border-white/10 rounded-lg px-2 py-1.5 text-xs text-white outline-none" />
              <button onClick={createTemplateDraft} disabled={creatingTemplate || !templateName.trim()}
                className="w-full py-1.5 rounded-lg bg-purple-600 hover:bg-purple-500 disabled:opacity-40 text-white text-xs">
                {creatingTemplate ? 'Creating...' : 'Create sandbox draft'}
              </button>
              {templateMessage && <div className="text-[10px] text-gray-400">{templateMessage}</div>}
              {editingTemplateId !== null && (
                <AgentTemplateEditor
                  agentId={agentId}
                  templateId={editingTemplateId}
                  templateName={templateDrafts.find((draft) => draft.id === editingTemplateId)?.name || `Template #${editingTemplateId}`}
                  onClosed={() => setEditingTemplateId(null)}
                  onSaved={() => { void loadTemplateDrafts(); }}
                />
              )}
              <div className="space-y-1">
                {templateDrafts.map((draft) => (
                  <div key={draft.id} className="rounded-lg bg-gray-800/60 px-2 py-1.5 text-xs">
                    <div className="flex items-center justify-between">
                      <span className="text-gray-200">#{draft.id} {draft.name}</span>
                      <span className="text-gray-500">{draft.status}</span>
                    </div>
                    {draft.review_reason && (
                      <div className="mt-1 text-[10px] text-gray-500">Review: {draft.review_reason}</div>
                    )}
                    <div className="mt-1 flex gap-1">
                      <button onClick={() => simulateTemplateDraft(draft.id)} disabled={simulatingTemplate === draft.id}
                        className="flex-1 rounded bg-blue-600/80 px-2 py-1 text-[10px] text-white hover:bg-blue-500 disabled:opacity-40">
                        {simulatingTemplate === draft.id ? 'Running...' : 'Simulate'}
                      </button>
                      {(draft.status === 'draft' || draft.status === 'rejected') && (
                        <button onClick={() => setEditingTemplateId(draft.id)}
                          className="flex-1 rounded bg-purple-600/80 px-2 py-1 text-[10px] text-white hover:bg-purple-500">
                          Edit
                        </button>
                      )}
                      {(draft.status === 'draft' || draft.status === 'rejected') && (
                        <button onClick={() => submitTemplateDraft(draft.id)} disabled={submittingTemplate === draft.id}
                          className="flex-1 rounded bg-emerald-600/80 px-2 py-1 text-[10px] text-white hover:bg-emerald-500 disabled:opacity-40">
                          {submittingTemplate === draft.id ? 'Sending...' : 'Submit'}
                        </button>
                      )}
                    </div>
                    {Array.isArray(draft.simulation_trace?.steps) && (
                      <div className="mt-1 text-[10px] text-gray-500">
                        {String(draft.simulation_trace.status)}: {(draft.simulation_trace.steps as Array<{ node_id?: string; say?: string }>)
                          .map((step) => step.node_id || step.say)
                          .filter(Boolean)
                          .join(' → ')}
                      </div>
                    )}
                  </div>
                ))}
              </div>
            </div>
          )}

          {tab === 'world' && (
            <div className="p-4 space-y-3 text-xs">
              <div className="rounded-xl border border-white/10 bg-gray-800/60 p-3">
                <div className="text-gray-400 mb-2">A2A movement</div>
                <div className="flex gap-2">
                  <input value={tileId} onChange={(e) => setTileId(e.target.value)}
                    className="flex-1 bg-gray-900/80 border border-white/10 rounded-lg px-2 py-1.5 text-white outline-none" />
                  <button onClick={() => move(tileId)} disabled={busy}
                    className="px-3 rounded-lg bg-blue-600 hover:bg-blue-500 disabled:opacity-40 text-white">Go</button>
                </div>
              </div>
              <div className="grid grid-cols-3 gap-2">
                {['tile_0_0', 'tile_1_0', 'tile_-1_0', 'tile_0_1', 'tile_0_-1'].map((tile) => (
                  <button key={tile} onClick={() => move(tile)} disabled={busy}
                    className="rounded-lg bg-gray-800 hover:bg-gray-700 text-gray-200 py-1.5">{tile.replace('tile_', '')}</button>
                ))}
              </div>
              <p className="text-gray-500 leading-relaxed">Movement is sent through the A2A action endpoint, not the human WebSocket. The map updates from the same world broadcast.</p>
            </div>
          )}

          {tab === 'behavior' && (
            <div className="p-4 space-y-3 text-xs overflow-y-auto min-h-0" style={{ maxHeight: 'calc(100vh - 170px)' }}>
              <div className="flex gap-2">
                <select value={selectedNpc} onChange={(e) => setSelectedNpc(e.target.value)}
                  className="flex-1 bg-gray-800/80 border border-white/10 rounded-lg px-2 py-1.5 text-white outline-none">
                  {cards.filter((c) => c.agent_id.startsWith('npc_')).map((c) => (
                    <option key={c.agent_id} value={c.agent_id}>{c.name || c.agent_id}</option>
                  ))}
                </select>
                <button onClick={() => loadBehavior()} disabled={busy}
                  className="px-3 rounded-lg bg-purple-600 hover:bg-purple-500 disabled:opacity-40 text-white">Load</button>
              </div>
              {behavior ? (
                <>
                  <div className="rounded-xl border border-white/10 bg-gray-800/60 p-3">
                    <div className="text-gray-400">{behavior.name}</div>
                    <div className="text-white mt-1">{behavior.nodes[behaviorNode]?.say || behavior.default_say}</div>
                  </div>
                  <div className="space-y-2">
                    {(behavior.nodes[behaviorNode]?.options || []).map((option) => (
                      <button key={option.id} onClick={() => runBehaviorNode(option.id)} disabled={busy}
                        className="w-full text-left rounded-lg bg-gray-800/80 hover:bg-gray-700 border border-white/5 px-3 py-2 text-gray-100">
                        {option.text}
                        <span className="block text-[10px] text-gray-500 mt-0.5">{option.id}</span>
                      </button>
                    ))}
                  </div>
                  <div className="rounded-xl border border-white/10 bg-gray-900/60 p-3">
                    <div className="text-gray-400 mb-2">Tree graph</div>
                    <div className="space-y-1">
                      {Object.entries(behavior.nodes).map(([nodeID, node]) => (
                        <div key={nodeID} className="flex items-center gap-2">
                          <span className={`w-1.5 h-1.5 rounded-full ${behaviorNode === nodeID ? 'bg-purple-400' : nodeID === behavior.initial ? 'bg-emerald-400' : 'bg-gray-600'}`} />
                          <button onClick={() => runBehaviorNode(nodeID)} className="text-gray-300 hover:text-white">{nodeID}</button>
                          <span className="text-gray-600">→ {node.options.length || 'end'}</span>
                        </div>
                      ))}
                    </div>
                  </div>
                </>
              ) : (
                <div className="text-gray-500 text-center py-6">Load an NPC to inspect its talk tree</div>
              )}
            </div>
          )}

          <div ref={chatRef} className="flex-1 overflow-y-auto px-4 pb-2 space-y-2 min-h-0" style={{ maxHeight: '45vh' }}>
            {messages.length === 0 && (
              <div className="text-gray-500 text-xs text-center py-8">Select NPC and send a message</div>
            )}
            {visibleMessages.map((m, i) => (
              <div key={i} className={`text-xs ${m.role === 'agent' ? 'text-right' : 'text-left'}`}>
                <span className={`font-medium ${m.role === 'agent' ? 'text-indigo-400' : m.role === 'npc' ? 'text-emerald-400' : 'text-gray-500'}`}>
                  {m.from}
                </span>
                <div className={`inline-block rounded-lg px-2.5 py-1.5 mt-0.5 max-w-[85%] ${
                  m.role === 'agent' ? 'bg-indigo-600/80 text-white rounded-br-none' :
                  m.role === 'npc' ? 'bg-gray-700/80 text-gray-100 rounded-bl-none' :
                  'bg-transparent text-gray-500'
                }`}>
                  {m.text}
                </div>
                {(m.messageId || m.traceId) && (
                  <div className={`text-[10px] text-gray-600 mt-1 ${m.role === 'agent' ? 'mr-1' : 'ml-1'}`}>
                    {m.messageId ? `#${m.messageId.slice(-8)}` : ''}
                    {m.messageId && m.traceId ? ' · ' : ''}
                    {m.traceId ? `trace:${m.traceId.slice(-8)}` : ''}
                  </div>
                )}
              </div>
            ))}
          </div>

          <div className="flex border-t border-white/10">
            <input value={input} onChange={(e) => setInput(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && send()}
              className="flex-1 bg-transparent px-3 py-2.5 text-sm text-white placeholder-gray-500 outline-none"
              placeholder={selectedNpc ? `Message ${selectedNpc}...` : 'Select NPC first'} />
            <button onClick={send} disabled={sending || !input.trim()}
              className="px-4 bg-indigo-600 hover:bg-indigo-500 disabled:opacity-40 text-white text-sm transition">
              ➤
            </button>
          </div>
        </>
      )}
    </div>
  );
}
