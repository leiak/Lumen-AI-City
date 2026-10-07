import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { TransactionsClient } from '@/app/transactions/TransactionsClient';
import { usePlayerStore } from '@/lib/usePlayerStore';

const mockFetch = vi.fn();
vi.stubGlobal('fetch', mockFetch);

function renderWithQuery(ui: React.ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
}

describe('TransactionsClient', () => {
  beforeEach(() => {
    usePlayerStore.setState({ selectedPlayerId: null });
    mockFetch.mockReset();
  });

  it('renders transaction table on data', async () => {
    usePlayerStore.setState({ selectedPlayerId: 'alice-uuid' });
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        user_id: 'alice-uuid',
        total: 3,
        limit: 50,
        offset: 0,
        items: [
          { id: 1, type: 'player_transfer', currency: 'gold', amount: 100, balance_after: 900, created_at: '2026-10-01T10:00:00Z', counterparty_id: 'bob', memo: 'tip' },
          { id: 2, type: 'npc_purchase', currency: 'gold', amount: -50, balance_after: 850, created_at: '2026-10-02T11:00:00Z', product_id: 42 },
          { id: 3, type: 'central_bank_emit', currency: 'gold', amount: 50, balance_after: 900, created_at: '2026-10-03T12:00:00Z' },
        ],
      }),
    });
    renderWithQuery(<TransactionsClient />);
    await waitFor(() => {
      expect(screen.getByText('玩家转账')).toBeInTheDocument();
    });
    expect(screen.getByText('NPC 购买')).toBeInTheDocument();
    expect(screen.getByText('中央银行发钞')).toBeInTheDocument();
    // amount formatting: +100 / -50 / +50 (sign prefix for non-negative, formatted)
    expect(screen.getByText('+100')).toBeInTheDocument();
    expect(screen.getByText('-50')).toBeInTheDocument();
    expect(screen.getByText('+50')).toBeInTheDocument();
  });

  it('pagination next button increases offset', async () => {
    usePlayerStore.setState({ selectedPlayerId: 'alice-uuid' });
    // initial fetch returns total > limit so 翻页 button is enabled
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        user_id: 'alice-uuid',
        total: 100,
        limit: 50,
        offset: 0,
        items: [],
      }),
    });
    renderWithQuery(<TransactionsClient />);
    await waitFor(() => {
      expect(screen.getByRole('button', { name: /下一页/ })).toBeEnabled();
    });
    // clicking next should trigger another fetch with offset=50
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        user_id: 'alice-uuid',
        total: 100,
        limit: 50,
        offset: 50,
        items: [],
      }),
    });
    screen.getByRole('button', { name: /下一页/ }).click();
    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledTimes(2);
    });
  });
});
