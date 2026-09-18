/**
 * NPCDialog 组件单测 —— Sprint 12 T03c。
 *
 * 测试目标：
 *   - 监听 aicity:npc_dialogue CustomEvent 渲染浮动对话气泡
 *   - 点击 option 调 api.postNpcTalk(npcId, choiceId, playerId)
 *   - Esc 键关闭
 *   - 默认隐藏（payload 为 null）
 *
 * mock 策略（vitest jsdom env）：
 *   - vi.mock('@/lib/api', factory) 替身 api.postNpcTalk
 *   - 用 fireEvent 派发 aicity:npc_dialogue CustomEvent（detail = 完整信封）
 *   - jsdom 默认支持中文 getByText；如需可改 /来了您嘞！/ 正则
 */
// @vitest-environment jsdom
import { afterEach, describe, it, expect, vi, beforeEach } from 'vitest';
import { cleanup, render, screen, fireEvent, waitFor } from '@testing-library/react';

vi.mock('@/lib/api', () => ({
  api: {
    postNpcTalk: vi.fn(),
  },
}));

import { api } from '@/lib/api';
import { useGameStore } from '@/store/game';
import { NPCDialog } from './NPCDialog';

function fireDialogue(payload: {
  npc_id: string;
  player_id: string;
  tile_id: string;
  say: string;
  options: Array<{ id: string; text: string }>;
  reply_to_choice_id: string | null;
}) {
  fireEvent(
    window,
    new CustomEvent('aicity:npc_dialogue', {
      detail: {
        type: 'npc_dialogue',
        trace_id: 't-1',
        ts_ms: Date.now(),
        payload,
      },
    }),
  );
}

describe('NPCDialog (T03c)', () => {
  beforeEach(() => {
    vi.mocked(api.postNpcTalk).mockReset();
    useGameStore.setState({ playerId: '' });
  });

  // vitest globals=false：手动 cleanup，否则跨 test 渲染会污染 body
  // (findByText 看到上一个 test 残留的 dialog → 匹配多个 → timeout)
  afterEach(() => {
    cleanup();
  });

  it('does not render when no npc_dialogue event has fired', () => {
    render(<NPCDialog />);
    expect(screen.queryByRole('dialog')).toBeNull();
  });

  it('renders when aicity:npc_dialogue fires', async () => {
    render(<NPCDialog />);
    fireDialogue({
      npc_id: 'npc_wang_boss_001',
      player_id: '',
      tile_id: '',
      say: '来了您嘞！',
      options: [{ id: 'ask_business', text: '问生意' }],
      reply_to_choice_id: null,
    });

    expect(await screen.findByText('来了您嘞！')).toBeInTheDocument();
    expect(screen.getByText('问生意')).toBeInTheDocument();
    // 角色是 dialog
    expect(screen.getByRole('dialog')).toBeInTheDocument();
  });

  it('clicking option calls api.postNpcTalk with npcId/choiceId/playerId', async () => {
    vi.mocked(api.postNpcTalk).mockResolvedValue({
      npc_id: 'npc_wang_boss_001',
      player_id: 'player-1',
      tile_id: 'tile_0_0',
      say: '来我店看看？',
      options: [],
      reply_to_choice_id: 'ask_business',
    });

    render(<NPCDialog />);
    fireDialogue({
      npc_id: 'npc_wang_boss_001',
      player_id: 'player-1',
      tile_id: 'tile_0_0',
      say: '啥事？',
      options: [{ id: 'ask_business', text: '问生意' }],
      reply_to_choice_id: null,
    });

    const btn = await screen.findByText('问生意');
    fireEvent.click(btn);

    await waitFor(() => {
      expect(api.postNpcTalk).toHaveBeenCalledWith(
        'npc_wang_boss_001',
        'ask_business',
        'player-1',
      );
    });
  });

  it('uses signed-in player id for replies when payload player_id is empty', async () => {
    useGameStore.setState({ playerId: 'store-player-9' });
    vi.mocked(api.postNpcTalk).mockResolvedValue({
      npc_id: 'npc_wang_boss_001',
      player_id: 'store-player-9',
      tile_id: 'tile_0_0',
      say: '好嘞。',
      options: [],
      reply_to_choice_id: 'ask_food',
    });

    render(<NPCDialog />);
    // active say：payload.player_id === ''，options 由 agent-os 上游携带
    fireDialogue({
      npc_id: 'npc_wang_boss_001',
      player_id: '',
      tile_id: '',
      say: '来了您嘞！',
      options: [{ id: 'ask_food', text: '有什么招牌菜？' }],
      reply_to_choice_id: null,
    });

    const btn = await screen.findByText('有什么招牌菜？');
    fireEvent.click(btn);

    await waitFor(() => {
      expect(api.postNpcTalk).toHaveBeenCalledWith(
        'npc_wang_boss_001',
        'ask_food',
        'store-player-9',
      );
    });
  });


  it('renders the /v1/npc/talk reply directly from the sync response', async () => {
    vi.mocked(api.postNpcTalk).mockResolvedValue({
      npc_id: 'npc_wang_boss_001',
      player_id: 'player-1',
      tile_id: 'tile_0_0',
      say: '老北京炸酱面，十八块一碗。',
      options: [{ id: 'leave', text: '来一碗' }],
      reply_to_choice_id: 'ask_food',
    });

    render(<NPCDialog />);
    fireDialogue({
      npc_id: 'npc_wang_boss_001',
      player_id: 'player-1',
      tile_id: 'tile_0_0',
      say: '来了您嘞！',
      options: [{ id: 'ask_food', text: '有什么招牌菜？' }],
      reply_to_choice_id: null,
    });

    const btn = await screen.findByText('有什么招牌菜？');
    fireEvent.click(btn);

    // 无需等待 WS 事件 —— 后端同步响应直接驱动渲染
    await waitFor(() => {
      expect(screen.getByText('老北京炸酱面，十八块一碗。')).toBeInTheDocument();
    });
    expect(screen.getByText('来一碗')).toBeInTheDocument();
  });
  it('closes on Esc key', async () => {

    render(<NPCDialog />);
    fireDialogue({
      npc_id: 'npc_x',
      player_id: '',
      tile_id: '',
      say: 'hi',
      options: [],
      reply_to_choice_id: null,
    });

    await screen.findByText('hi');
    fireEvent.keyDown(window, { key: 'Escape' });

    await waitFor(() => {
      expect(screen.queryByText('hi')).toBeNull();
      expect(screen.queryByRole('dialog')).toBeNull();
    });
  });
  it('suppresses NPC dialogue addressed to another player', () => {
    useGameStore.setState({ playerId: 'store-player-9' });
    render(<NPCDialog />);
    // welcome 专属台词带了目标玩家 id，但当前客户端是 store-player-9
    fireDialogue({
      npc_id: 'npc_wang_boss_001',
      player_id: 'other-player',
      tile_id: 'tile_0_0',
      say: '佣人来了！',
      options: [],
      reply_to_choice_id: null,
    });

    expect(screen.queryByRole('dialog')).toBeNull();
  });

  it('renders self-addressed welcome when signed in', async () => {
    useGameStore.setState({ playerId: 'store-player-9' });
    render(<NPCDialog />);
    fireDialogue({
      npc_id: 'npc_wang_boss_001',
      player_id: 'store-player-9',
      tile_id: 'tile_0_0',
      say: '哟，您可算回来了！',
      options: [{ id: 'ask_food', text: '有什么菜？' }],
      reply_to_choice_id: null,
    });

    expect(await screen.findByText('哟，您可算回来了！')).toBeInTheDocument();
    expect(screen.getByText('有什么菜？')).toBeInTheDocument();
  });

  it('broadcast active say still shows when signed in', async () => {
    useGameStore.setState({ playerId: 'store-player-9' });
    render(<NPCDialog />);
    fireDialogue({
      npc_id: 'npc_wang_boss_001',
      player_id: '',
      tile_id: '',
      say: '欢迎光临小店。',
      options: [],
      reply_to_choice_id: null,
    });

    expect(await screen.findByText('欢迎光临小店。')).toBeInTheDocument();
  });
});
