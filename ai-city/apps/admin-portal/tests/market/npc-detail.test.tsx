import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { NpcTemplateDetail } from '@/app/market/npc-templates/[id]/NpcTemplateDetail';

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

describe('NpcTemplateDetail', () => {
  it('renders OCEAN radar, products, and purchase action', async () => {
    mockFetch.mockResolvedValueOnce(
      jsonResponse(200, {
        id: 41,
        name: 'Merchant NPC',
        status: 'live',
        price_gold: 120,
        ocean_json: { O: 0.8, C: 0.6, E: 0.4, A: 0.9, N: 0.2 },
        bt_skeleton: '{"type":"selector"}',
        product_catalog: [
          { name: 'Greeting', price_gold: 10 },
          { name: 'Report', price_gold: 25, stock: 3 },
        ],
      }),
    );

    renderWithQuery(<NpcTemplateDetail templateId={41} />);

    expect(await screen.findByRole('heading', { name: 'Merchant NPC' })).toBeInTheDocument();
    expect(screen.getByTestId('ocean-radar')).toBeInTheDocument();
    expect(screen.getAllByText(/^[OCEAN]$/)).toHaveLength(5);
    expect(screen.getByText('Greeting')).toBeInTheDocument();
    expect(screen.getByText('Report')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: '购买' })).toBeInTheDocument();
  });

  it('posts a purchase with an idempotency key', async () => {
    mockFetch.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (
        url === '/api/marketplace/npc-templates/41' &&
        (!init || init.method === undefined || init.method === 'GET')
      ) {
        return jsonResponse(200, {
          id: 41,
          name: 'Merchant NPC',
          status: 'live',
          price_gold: 120,
          ocean_json: { O: 0.5, C: 0.5, E: 0.5, A: 0.5, N: 0.5 },
          product_catalog: [],
        });
      }
      if (url === '/api/marketplace/purchase' && init?.method === 'POST') {
        return jsonResponse(200, { purchase_id: 71 });
      }
      throw new Error(`unexpected fetch: ${url}`);
    });

    renderWithQuery(<NpcTemplateDetail templateId={41} />);
    fireEvent.click(await screen.findByRole('button', { name: '购买' }));

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        '/api/marketplace/purchase',
        expect.objectContaining({ method: 'POST' }),
      );
      expect(screen.getByText('购买成功')).toBeInTheDocument();
    });
    const body = JSON.parse(mockFetch.mock.calls[1][1].body);
    expect(body).toMatchObject({ template_kind: 'npc', template_id: 41 });
    expect(body.idempotency_key).toBeTruthy();
  });

  it('shows a readable detail error', async () => {
    mockFetch.mockResolvedValueOnce(
      jsonResponse(404, {
        detail: { code: 'R_028', msg: 'template not found' },
      }),
    );

    renderWithQuery(<NpcTemplateDetail templateId={99} />);

    await waitFor(() => {
      expect(screen.getByText(/R_028/)).toBeInTheDocument();
      expect(screen.getByText(/template not found/)).toBeInTheDocument();
    });
  });

  it('shows a readable purchase error', async () => {
    mockFetch.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      if (
        url === '/api/marketplace/npc-templates/41' &&
        (!init || init.method === undefined || init.method === 'GET')
      ) {
        return jsonResponse(200, {
          id: 41,
          name: 'Merchant NPC',
          status: 'live',
          price_gold: 120,
          ocean_json: { O: 0.5, C: 0.5, E: 0.5, A: 0.5, N: 0.5 },
          product_catalog: [],
        });
      }
      if (url === '/api/marketplace/purchase' && init?.method === 'POST') {
        return jsonResponse(402, {
          detail: { code: 'R_022', msg: 'no gold' },
        });
      }
      throw new Error(`unexpected fetch: ${url}`);
    });

    renderWithQuery(<NpcTemplateDetail templateId={41} />);
    fireEvent.click(await screen.findByRole('button', { name: '购买' }));

    await waitFor(() => {
      expect(screen.getByText(/R_022/)).toBeInTheDocument();
      expect(screen.getByText(/no gold/)).toBeInTheDocument();
    });
  });
});
