import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { InventoryClient } from '@/app/inventory/InventoryClient';

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

describe('InventoryClient', () => {
  it('renders purchased templates for the signed-in admin', async () => {
    mockFetch.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url === '/api/auth/me') {
        return jsonResponse(200, { username: 'admin', role: 'admin' });
      }
      if (url.startsWith('/api/marketplace/inventory/admin')) {
        return jsonResponse(200, [
          {
            purchase_id: 71,
            template_kind: 'npc',
            template_id: 41,
            price_paid_gold: 120,
            created_at: '2026-10-08T00:00:00Z',
          },
        ]);
      }
      throw new Error(`unexpected fetch: ${url}`);
    });

    renderWithQuery(<InventoryClient />);

    await waitFor(() => {
      expect(screen.getByTestId('inventory-row-71')).toHaveTextContent('npc');
      expect(screen.getByTestId('inventory-row-71')).toHaveTextContent('41');
      expect(screen.getByTestId('inventory-row-71')).toHaveTextContent('120');
    });
    expect(mockFetch).toHaveBeenCalledWith(
      '/api/marketplace/inventory/admin?limit=50&offset=0',
      expect.anything(),
    );
  });

  it('shows an empty inventory', async () => {
    mockFetch.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url === '/api/auth/me') {
        return jsonResponse(200, { username: 'admin', role: 'admin' });
      }
      if (url.startsWith('/api/marketplace/inventory/admin')) {
        return jsonResponse(200, []);
      }
      throw new Error(`unexpected fetch: ${url}`);
    });

    renderWithQuery(<InventoryClient />);

    expect(await screen.findByText('暂无已购模板')).toBeInTheDocument();
  });

  it('shows a readable API error', async () => {
    mockFetch.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url === '/api/auth/me') {
        return jsonResponse(200, { username: 'admin', role: 'admin' });
      }
      if (url.startsWith('/api/marketplace/inventory/admin')) {
        return jsonResponse(403, {
          error: { code: 'R_026', msg: 'admin role required' },
        });
      }
      throw new Error(`unexpected fetch: ${url}`);
    });

    renderWithQuery(<InventoryClient />);

    await waitFor(() => {
      expect(screen.getByText(/R_026/)).toBeInTheDocument();
      expect(screen.getByText(/admin role required/)).toBeInTheDocument();
    });
  });
});
