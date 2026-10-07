import { describe, it, expect, vi } from 'vitest';
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
  it('renders gold + token balance cards on data', async () => {
    usePlayerStore.setState({ selectedPlayerId: 'alice-uuid' });
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => ({ user_id: 'alice-uuid', gold_balance: 1000, token_balance: 50 }),
    });
    renderWithQuery(<WalletClient />);
    await waitFor(() => {
      expect(screen.getByText(/1,000|1000/)).toBeInTheDocument();
    });
    expect(screen.getByText(/50/)).toBeInTheDocument();
  });

  it('shows prompt when no player selected', () => {
    usePlayerStore.setState({ selectedPlayerId: null });
    renderWithQuery(<WalletClient />);
    expect(screen.getByText(/请从顶部选择 player/)).toBeInTheDocument();
  });
});