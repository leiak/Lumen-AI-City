import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { PlayerSelector } from '@/components/PlayerSelector';

const mockFetch = vi.fn();
vi.stubGlobal('fetch', mockFetch);

function renderWithQuery(ui: React.ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
}

describe('PlayerSelector', () => {
  it('renders dropdown options from /api/players', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        players: [
          { id: 'uuid-1', username: 'demo', role: 'player' },
          { id: 'uuid-2', username: 'admin', role: 'admin' },
        ],
      }),
    });
    renderWithQuery(<PlayerSelector />);
    await waitFor(() => {
      expect(screen.getByRole('option', { name: /demo/ })).toBeInTheDocument();
    });
    expect(screen.getByRole('option', { name: /admin/ })).toBeInTheDocument();
  });

  it('calls setSelected when user picks a player', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        players: [{ id: 'uuid-1', username: 'demo', role: 'player' }],
      }),
    });
    const { usePlayerStore } = await import('@/lib/usePlayerStore');
    usePlayerStore.setState({ selectedPlayerId: null });
    renderWithQuery(<PlayerSelector />);
    await waitFor(() => screen.getByRole('option', { name: /demo/ }));
    fireEvent.change(screen.getByRole('combobox'), {
      target: { value: 'uuid-1' },
    });
    await waitFor(() => {
      expect(usePlayerStore.getState().selectedPlayerId).toBe('uuid-1');
    });
  });
});