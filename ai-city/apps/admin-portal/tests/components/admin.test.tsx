import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor, fireEvent } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { AdminClient } from '@/app/admin/AdminClient';

const mockFetch = vi.fn();
vi.stubGlobal('fetch', mockFetch);

function renderWithQuery(ui: React.ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
}

describe('AdminClient', () => {
  beforeEach(() => {
    mockFetch.mockReset();
  });

  it('emits and invalidates wallet+transactions queries on success', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => ({ emitted_amount_per_player: 50, active_player_count: 1 }),
    });

    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    // Pre-populate wallet+transactions cache so we can assert invalidation
    qc.setQueryData(['wallet', 'alice-uuid'], { gold_balance: 1000 });
    qc.setQueryData(['transactions', 'alice-uuid', 50, 0], { items: [] });

    render(
      <QueryClientProvider client={qc}>
        <AdminClient />
      </QueryClientProvider>
    );

    // fill reason (exact match for emit form's "Reason" label — sink has "Reason (可选)")
    const reasonInput = screen.getByLabelText('Reason');
    fireEvent.change(reasonInput, { target: { value: 'daily_emit' } });

    // submit
    const submitBtn = screen.getByRole('button', { name: /触发 emit/ });
    submitBtn.click();

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith('/api/economy/admin/emit', expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ reason: 'daily_emit' }),
      }));
    });

    await waitFor(() => {
      expect(qc.getQueryState(['wallet', 'alice-uuid'])?.isInvalidated).toBe(true);
      expect(qc.getQueryState(['transactions', 'alice-uuid', 50, 0])?.isInvalidated).toBe(true);
    });
  });

  it('shows server error code on 4xx', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 403,
      json: async () => ({ error: { code: 'R_026', msg: 'admin role required' } }),
    });

    renderWithQuery(<AdminClient />);
    const reasonInput = screen.getByLabelText('Reason');
    fireEvent.change(reasonInput, { target: { value: 'daily_emit' } });
    screen.getByRole('button', { name: /触发 emit/ }).click();

    await waitFor(() => {
      expect(screen.getByText(/R_026/)).toBeInTheDocument();
    });
  });
});
