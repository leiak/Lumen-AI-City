// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';

vi.mock('@/lib/api', () => ({
  api: {
    listProducts: vi.fn().mockResolvedValue([
      {
        id: 1,
        npc_id: 'npc_wang_boss_001',
        name: '招牌红烧肉',
        price_gold: 50,
        price_token: null,
        stock: 1,
        enabled: true,
      },
    ]),
    purchaseProduct: vi.fn().mockResolvedValue({
      user_id: 'player-1',
      product_id: 1,
      currency: 'gold',
      amount_paid: 50,
      balance_after: 0,
      sink_amount: 2,
    }),
    getWallet: vi.fn().mockResolvedValue({ gold_balance: 0, token_balance: 0 }),
  },
}));

import { NpcShop } from './NpcShop';
import { useGameStore } from '@/store/game';

describe('NpcShop purchase UX', () => {
  afterEach(() => {
    cleanup();
  });

  it('checks sink-inclusive cost and shows actual debit', async () => {
    useGameStore.setState({
      sessionStatus: 'authenticated',
      playerId: 'player-1',
      wallet: { gold: 52, token: 0 },
    });
    render(<NpcShop npcId="npc_wang_boss_001" />);

    expect(await screen.findByText(/实扣 52 G/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: '购买' }));

    expect(await screen.findByText('已购买 招牌红烧肉 · 扣款 52 G')).toBeInTheDocument();
    expect(useGameStore.getState().wallet?.gold).toBe(0);
  });
});
