import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { NpcTemplateList } from '@/app/creator/npc-templates/NpcTemplateList';

const mockFetch = vi.fn();
vi.stubGlobal('fetch', mockFetch);

function renderWithQuery(ui: React.ReactNode) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(<QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>);
}

function jsonResponse(status: number, body: unknown) {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
  };
}

beforeEach(() => {
  mockFetch.mockReset();
});

describe('NpcTemplateList', () => {
  it('renders templates and the create action for a creator', async () => {
    mockFetch.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url === '/api/auth/me') {
        return jsonResponse(200, { username: 'creator', role: 'creator' });
      }
      if (url.startsWith('/api/marketplace/npc-templates')) {
        return jsonResponse(200, [
          {
            id: 41,
            name: 'Merchant NPC',
            status: 'live',
            price_gold: 120,
            created_at: '2026-10-08T00:00:00Z',
          },
          {
            id: 42,
            name: 'Guard NPC',
            status: 'taken_down',
            price_gold: 80,
            created_at: '2026-10-07T00:00:00Z',
          },
        ]);
      }
      throw new Error(`unexpected fetch: ${url}`);
    });

    renderWithQuery(<NpcTemplateList />);

    expect(await screen.findByRole('heading', { name: 'NPC Templates' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: '创建' })).toHaveAttribute(
      'href',
      '/creator/npc-templates/new',
    );
    expect(screen.getByTestId('npc-template-row-41')).toHaveTextContent('Merchant NPC');
    expect(screen.getByTestId('npc-template-row-42')).toHaveTextContent('Guard NPC');
    expect(screen.getByTestId('npc-template-row-41')).toHaveTextContent('120');
    expect(mockFetch).toHaveBeenCalledWith(
      '/api/marketplace/npc-templates?limit=50&offset=0&status=live',
      expect.anything(),
    );
  });

  it('hides the create action for players', async () => {
    mockFetch.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url === '/api/auth/me') {
        return jsonResponse(200, { username: 'player', role: 'player' });
      }
      if (url.startsWith('/api/marketplace/npc-templates')) {
        return jsonResponse(200, []);
      }
      throw new Error(`unexpected fetch: ${url}`);
    });

    renderWithQuery(<NpcTemplateList />);

    expect(await screen.findByText('暂无 NPC 模板')).toBeInTheDocument();
    expect(screen.queryByRole('link', { name: '创建' })).not.toBeInTheDocument();
  });

  it('shows an empty state', async () => {
    mockFetch.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url === '/api/auth/me') {
        return jsonResponse(200, { username: 'creator', role: 'creator' });
      }
      if (url.startsWith('/api/marketplace/npc-templates')) {
        return jsonResponse(200, []);
      }
      throw new Error(`unexpected fetch: ${url}`);
    });

    renderWithQuery(<NpcTemplateList />);

    expect(await screen.findByText('暂无 NPC 模板')).toBeInTheDocument();
  });

  it('shows a readable API error', async () => {
    mockFetch.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url === '/api/auth/me') {
        return jsonResponse(200, { username: 'creator', role: 'creator' });
      }
      if (url.startsWith('/api/marketplace/npc-templates')) {
        return jsonResponse(403, {
          error: { code: 'R_027', msg: 'insufficient role' },
        });
      }
      throw new Error(`unexpected fetch: ${url}`);
    });

    renderWithQuery(<NpcTemplateList />);

    await waitFor(() => {
      expect(screen.getByText(/R_027/)).toBeInTheDocument();
      expect(screen.getByText(/insufficient role/)).toBeInTheDocument();
    });
  });

  it('shows a loading state before requests resolve', () => {
    mockFetch.mockImplementation(() => new Promise(() => {}));

    renderWithQuery(<NpcTemplateList />);

    expect(screen.getByText('加载中…')).toBeInTheDocument();
  });
});
