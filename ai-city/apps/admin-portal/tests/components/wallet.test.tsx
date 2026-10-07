import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { WalletClient } from '@/app/wallet/WalletClient';
import { usePlayerStore } from '@/lib/usePlayerStore';

const mockFetch = vi.fn();
vi.stubGlobal('fetch', mockFetch);

function renderWithQuery(ui: React.ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
}

describe('WalletClient', () => {
  beforeEach(() => {
    usePlayerStore.setState({ selectedPlayerId: null });
    mockFetch.mockReset();
  });

  it('renders gold + token balance cards on data', async () => {
    usePlayerStore.setState({ selectedPlayerId: 'alice-uuid' });
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => ({ user_id: 'alice-uuid', gold_balance: 1000, token_balance: 50 }),
    });
    renderWithQuery(<WalletClient />);
    await waitFor(() => {
      expect(screen.getByTestId('balance-gold-balance')).toHaveTextContent('1,000');
    });
    expect(screen.getByTestId('balance-token-balance')).toHaveTextContent('50');
  });

  it('shows prompt when no player selected', () => {
    usePlayerStore.setState({ selectedPlayerId: null });
    renderWithQuery(<WalletClient />);
    expect(screen.getByText(/请从顶部选择 player/)).toBeInTheDocument();
  });

  it('shows error banner on non-2xx response', async () => {
    usePlayerStore.setState({ selectedPlayerId: 'alice-uuid' });
    // query uses retry: 1, so mock both attempts
    mockFetch.mockResolvedValue({
      ok: false,
      status: 500,
      json: async () => ({ error: { code: 'R_502', msg: 'economy-service unreachable' } }),
    });
    renderWithQuery(<WalletClient />);
    await waitFor(
      () => {
        expect(screen.getByText(/R_502/)).toBeInTheDocument();
      },
      { timeout: 5000 }
    );
  });

  it('shows loading state initially', () => {
    usePlayerStore.setState({ selectedPlayerId: 'alice-uuid' });
    // don't resolve fetch
    mockFetch.mockReturnValueOnce(new Promise(() => {}));
    renderWithQuery(<WalletClient />);
    expect(screen.getByText(/加载中/)).toBeInTheDocument();
  });
});