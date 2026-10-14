// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';

import { AgentTemplateEditor } from './AgentTemplateEditor';

const editorTree = {
  id: 'root',
  type: 'sequence',
  children: [{ id: 'entrypoint', type: 'action', name: 'set_npc_state', args: ['dialogue_entry', 'root'] }],
};

const editorPayload = {
  id: 7,
  agent_id: 'agent-1',
  name: 'Helper',
  status: 'draft',
  version: 1,
  talk_tree: { initial: 'root', nodes: { root: { say: 'Hello', options: [] } } },
  bt_editor_tree: editorTree,
  node_edges: [],
};

describe('AgentTemplateEditor', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (url.includes('/editor/revisions')) {
        return Response.json({ revisions: [{ version: 1, edited_by: 'agent-1', created_at: 'now' }] });
      }
      if (url.endsWith('/editor') && init?.method === 'PUT') {
        return Response.json({ ...editorPayload, version: 2 });
      }
      return Response.json(editorPayload);
    }));
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it('loads and saves the agent draft through the BT editor contract', async () => {
    const onSaved = vi.fn();
    render(
      <AgentTemplateEditor
        agentId="agent-1"
        templateId={7}
        templateName="Helper"
        onClosed={() => {}}
        onSaved={onSaved}
      />,
    );

    expect(await screen.findByText(/Draft #7 loaded at version 1/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Save version' }));

    await waitFor(() => expect(onSaved).toHaveBeenCalledTimes(1));
    expect(await screen.findByText('Saved version 2')).toBeInTheDocument();
  });
});
