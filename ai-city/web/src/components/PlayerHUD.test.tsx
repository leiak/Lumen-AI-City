// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';

vi.mock('@/lib/api', () => ({
  api: {
    setToken: vi.fn(),
    getWallet: vi.fn().mockResolvedValue({
      gold_balance: 100,
      token_balance: 20,
    }),
    getTransactions: vi.fn().mockResolvedValue({ transactions: [], total: 0 }),
    getInventory: vi.fn().mockResolvedValue([
      {
        product_id: 1,
        name: '招牌红烧肉',
        quantity: 3,
        currencies: 'gold',
        last_purchased_at: null,
      },
    ]),
  },
}));

import { PlayerHUD } from './PlayerHUD';
import { useGameStore } from '@/store/game';

describe('PlayerHUD inventory', () => {
  beforeEach(() => {
    window.localStorage.clear();
    useGameStore.getState().setAuthenticatedPlayer({
      playerId: 'player-1',
      username: 'demo',
      displayName: 'Demo Player',
    });
  });

  afterEach(() => {
    cleanup();
  });

  it('shows aggregated NPC purchases as inventory', async () => {
    render(<PlayerHUD />);

    fireEvent.click(screen.getByRole('button', { name: '背包' }));

    expect(await screen.findByText('招牌红烧肉')).toBeInTheDocument();
    expect(screen.getByText('x3')).toBeInTheDocument();
  });
});
