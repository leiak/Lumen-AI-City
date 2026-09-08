/**
 * WorldMap NPC 命中交互单测 —— Sprint 12 T03d。
 *
 * 测试目标：
 *   - mount 后渲染 tile.npc_ids 的 NPC 圆点（带 data-npc-id）
 *   - 点击 NPC 圆点 → 派发 aicity:npc_dialogue CustomEvent（信封 payload 含
 *     npc_id / player_id / tile_id / say / options / reply_to_choice_id）
 *   - 命中 NPC 时不发 move（不调 api.move）
 *
 * mock 策略（vitest jsdom env）：
 *   - vi.mock('@/lib/api', factory) 替身 api.getTiles / api.move
 *   - localStorage.aicity_player_id 设值，game store 启动时同步进 playerId
 *     （避免 onSvgClick 第一关"未登录"早返回）
 *   - 直接 fireEvent.click(<circle data-npc-id>) —— React 事件委托会把
 *     e.target 设为 circle；closest('[data-npc-id]') 命中 circle 自身
 *
 * waitFor 陷阱：
 *   - waitFor 只在回调抛错时重试，回调返回 null 不会触发重试
 *   - 因此回调里必须主动 throw，未渲染好就抛 ElementNotFound
 */
// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  cleanup,
  fireEvent,
  render,
  waitFor,
} from '@testing-library/react';

vi.mock('@/lib/api', () => ({
  api: {
    getTiles: vi.fn(),
    move: vi.fn(),
    setToken: vi.fn(),
  },
}));

import { api } from '@/lib/api';
import { WorldMap } from './WorldMap';
import { NPC_DIALOGUE_EVENT } from '@/lib/ws-events';
import { useGameStore } from '@/store/game';

const SAMPLE_TILES = [
  {
    id: 'tile_1_1',
    center_x: 150,
    center_y: 150,
    size: 100,
    lod_level: 'CBD' as const,
    buildings: [],
    npc_ids: ['npc_wang_boss_001'],
    player_ids: ['test-player-id'], // 自己也在这个 tile，让 setPosition 初始化
  },
  {
    id: 'tile_0_0',
    center_x: 50,
    center_y: 50,
    size: 100,
    lod_level: 'Residential' as const,
    buildings: [],
    npc_ids: [],
    player_ids: [],
  },
];

function findNpc(): Element {
  const el = document.querySelector('[data-npc-id="npc_wang_boss_001"]');
  if (!el) throw new Error('NPC element not rendered yet');
  return el;
}

describe('WorldMap - NPC 命中 (T03d)', () => {
  let onDialogue: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    window.localStorage.setItem('aicity_player_id', 'test-player-id');
    // 直接设 store 跳过 200ms 轮询：WorldMap 的 playerId 兜底轮询是为
    // login 跳 city 的极小窗口设的，测试不需要等
    useGameStore.getState().setPlayerId('test-player-id');
    vi.mocked(api.getTiles).mockResolvedValue(SAMPLE_TILES);
    vi.mocked(api.move).mockResolvedValue({
      player_id: 'test-player-id',
      current_tile_id: 'tile_1_1',
      x: 150,
      y: 150,
      ts_ms: Date.now(),
      accepted: true,
    });
    onDialogue = vi.fn();
    window.addEventListener(NPC_DIALOGUE_EVENT, onDialogue);
  });

  afterEach(() => {
    window.removeEventListener(NPC_DIALOGUE_EVENT, onDialogue);
    cleanup();
    window.localStorage.clear();
    vi.restoreAllMocks();
  });

  it('渲染 NPC 圆点（带 data-npc-id 属性）', async () => {
    render(<WorldMap />);
    await waitFor(findNpc);
    expect(findNpc()).not.toBeNull();
  });

  it('点击 NPC 圆点派发 aicity:npc_dialogue CustomEvent', async () => {
    render(<WorldMap />);
    const npc = await waitFor(findNpc);

    fireEvent.click(npc);

    await waitFor(() => {
      expect(onDialogue).toHaveBeenCalledTimes(1);
    });

    const ev = onDialogue.mock.calls[0][0] as CustomEvent;
    expect(ev.type).toBe(NPC_DIALOGUE_EVENT);
    const detail = ev.detail;
    expect(detail.type).toBe('npc_dialogue');
    expect(typeof detail.trace_id).toBe('string');
    expect(detail.trace_id.length).toBeGreaterThan(0);
    expect(typeof detail.ts_ms).toBe('number');
    expect(detail.payload).toMatchObject({
      npc_id: 'npc_wang_boss_001',
      player_id: 'test-player-id',
      tile_id: 'tile_1_1',
      say: '王老板在。',
      reply_to_choice_id: null,
    });
    expect(detail.payload.options).toEqual([
      { id: 'greet', text: '打招呼' },
      { id: 'leave', text: '离开' },
    ]);
  });

  it('点中 NPC 时不发 move（仅开 dialog，不触发移动）', async () => {
    render(<WorldMap />);
    const npc = await waitFor(findNpc);

    fireEvent.click(npc);

    await waitFor(() => {
      expect(onDialogue).toHaveBeenCalledTimes(1);
    });
    expect(api.move).not.toHaveBeenCalled();
  });
});
