// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';

vi.mock('@/lib/api', () => ({
  api: {
    postNpcTalk: vi.fn().mockResolvedValue({
      say: '欢迎来到小镇。',
      options: [{ id: 'ask_shop', text: '有什么商品？' }],
    }),
  },
}));

import { ChatBox } from './ChatBox';
import { api } from '@/lib/api';
import { useGameStore } from '@/store/game';

describe('ChatBox', () => {
  beforeEach(() => {
    useGameStore.getState().selectNpc(null);
    useGameStore.getState().setPlayerId('player-1');
  });

  afterEach(() => {
    cleanup();
  });

  it('requires an NPC selection before sending', () => {
    render(<ChatBox />);

    expect(screen.getByText('点击地图上的 NPC 开始对话。')).toBeInTheDocument();
    expect(screen.getByPlaceholderText('先选中一个 NPC')).toBeDisabled();
    expect(screen.getByRole('button', { name: '发送' })).toBeDisabled();
  });

  it('sends messages to the selected NPC', async () => {
    useGameStore.getState().selectNpc('npc_snack_owner_001');
    render(<ChatBox />);

    expect(screen.getByText('正在和 小吃摊老板 对话；点击选项或直接发送消息...')).toBeInTheDocument();
    fireEvent.change(screen.getByPlaceholderText('和 小吃摊老板 说点什么...'), {
      target: { value: '你好' },
    });
    fireEvent.click(screen.getByRole('button', { name: '发送' }));

    await waitFor(() => expect(api.postNpcTalk).toHaveBeenCalledWith(
      'npc_snack_owner_001',
      'root',
      'player-1',
    ));
    expect(await screen.findByText('欢迎来到小镇。')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: '有什么商品？' })).toBeInTheDocument();
  });
});
