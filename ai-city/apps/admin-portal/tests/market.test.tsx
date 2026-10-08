import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MarketClient } from '@/app/market/MarketClient';

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

describe('MarketClient', () => {
  it('renders NPC template cards by default', async () => {
    mockFetch.mockResolvedValueOnce(
      jsonResponse(200, [
        { id: 41, name: 'Merchant NPC', status: 'live', price_gold: 120 },
      ]),
    );

    renderWithQuery(<MarketClient />);

    expect(await screen.findByTestId('npc-card-41')).toHaveTextContent('Merchant NPC');
    expect(screen.getByTestId('npc-card-41')).toHaveTextContent('120');
    expect(screen.queryByTestId(/saga-card-/)).not.toBeInTheDocument();
    expect(mockFetch).toHaveBeenCalledWith(
      '/api/marketplace/npc-templates?limit=50&offset=0&status=live',
      expect.anything(),
    );
  });

  it('switches to saga templates', async () => {
    mockFetch.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes('saga-templates')) {
        return jsonResponse(200, [
          { id: 51, name: 'WelcomeSaga', status: 'live', price_gold: 100 },
        ]);
      }
      return jsonResponse(200, []);
    });

    renderWithQuery(<MarketClient />);
    await screen.findByTestId('market-empty');
    fireEvent.click(screen.getByRole('button', { name: 'Saga' }));

    expect(await screen.findByTestId('saga-card-51')).toHaveTextContent('WelcomeSaga');
    expect(mockFetch).toHaveBeenCalledWith(
      '/api/marketplace/saga-templates?limit=50&offset=0&status=live',
      expect.anything(),
    );
  });

  it('filters visible cards by search text', async () => {
    mockFetch.mockResolvedValueOnce(
      jsonResponse(200, [
        { id: 41, name: 'Merchant NPC', status: 'live', price_gold: 120 },
        { id: 42, name: 'Guard NPC', status: 'live', price_gold: 80 },
      ]),
    );

    renderWithQuery(<MarketClient />);
    await screen.findByTestId('npc-card-41');
    fireEvent.change(screen.getByLabelText('搜索'), {
      target: { value: 'guard' },
    });

    expect(screen.getByTestId('npc-card-42')).toBeInTheDocument();
    expect(screen.queryByTestId('npc-card-41')).not.toBeInTheDocument();
  });

  it('shows a readable API error', async () => {
    mockFetch.mockResolvedValueOnce(
      jsonResponse(502, {
        error: { code: 'R_502', msg: 'economy-service unreachable' },
      }),
    );

    renderWithQuery(<MarketClient />);

    await waitFor(() => {
      expect(screen.getByText(/R_502/)).toBeInTheDocument();
      expect(screen.getByText(/economy-service unreachable/)).toBeInTheDocument();
    });
  });
});
