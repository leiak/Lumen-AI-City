// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';

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
    transfer: vi.fn().mockResolvedValue({
      user_id: 'player-1',
      gold_balance: 90,
      token_balance: 20,
      created_at: '',
      updated_at: '',
    }),
    listNpcTemplates: vi.fn().mockResolvedValue([
      {
        id: 12,
        name: 'Creator NPC',
        price_gold: 30,
        creator_id: 'player-1',
        status: 'live',
      },
    ]),
    listSagaTemplates: vi.fn().mockResolvedValue([]),
    purchaseMarketplaceTemplate: vi.fn().mockResolvedValue({ purchase_id: 71 }),
    getCreatorRevenue: vi.fn().mockResolvedValue([
      {
        purchase_id: 71,
        amount_gold: 100,
        platform_cut_gold: 0,
        created_at: null,
      },
    ]),
    createNpcTemplate: vi.fn().mockResolvedValue({ id: 12 }),
    takeDownMarketTemplate: vi.fn().mockResolvedValue({ status: 'taken_down' }),
  },
}));

import { PlayerHUD } from './PlayerHUD';
import { useGameStore } from '@/store/game';
import { api } from '@/lib/api';

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

  it('submits gold transfer with target player', async () => {
    render(<PlayerHUD />);

    fireEvent.click(screen.getByRole('button', { name: '转账' }));
    fireEvent.change(screen.getByPlaceholderText('目标玩家 UUID'), {
      target: { value: '22222222-2222-4222-8222-222222222222' },
    });
    fireEvent.change(screen.getByPlaceholderText('Gold 数量'), {
      target: { value: '10' },
    });
    fireEvent.click(screen.getByRole('button', { name: '转账 Gold' }));

    await waitFor(() => expect(api.transfer).toHaveBeenCalledWith(expect.objectContaining({
      toUserId: '22222222-2222-4222-8222-222222222222',
      amount: 10,
    })));
    expect(await screen.findByText('转账成功')).toBeInTheDocument();
    expect(screen.getByText('Gold: 90 · Token: 20')).toBeInTheDocument();
  });

  it('shows creator marketplace templates for purchase', async () => {
    render(<PlayerHUD />);

    fireEvent.click(screen.getByRole('button', { name: '市场' }));

    expect(await screen.findByText(/Creator NPC/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: '30 G' })).toBeInTheDocument();
  });

  it('shows creator revenue summary when authorized', async () => {
    render(<PlayerHUD />);

    fireEvent.click(screen.getByRole('button', { name: '收益' }));

    expect(await screen.findByText('总收益: 100 Gold')).toBeInTheDocument();
    expect(screen.getByText('#71')).toBeInTheDocument();
  });

  it('publishes an NPC template from creator studio', async () => {
    render(<PlayerHUD />);

    fireEvent.click(screen.getByRole('button', { name: '发布' }));
    fireEvent.change(screen.getByPlaceholderText('作品名称'), {
      target: { value: 'Studio NPC' },
    });
    fireEvent.click(screen.getByRole('button', { name: '发布到市场' }));

    expect(await screen.findByText('已发布 NPC 模板 #12')).toBeInTheDocument();
  });

  it('lets creators take down their own marketplace template', async () => {
    render(<PlayerHUD />);

    fireEvent.click(screen.getByRole('button', { name: '市场' }));
    fireEvent.click(await screen.findByRole('button', { name: '下架' }));

    await waitFor(() => expect(api.takeDownMarketTemplate).toHaveBeenCalledWith({
      kind: 'npc',
      templateId: 12,
    }));
  });
});
