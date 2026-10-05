/**
 * WorldMap NPC 头像 + emoji fallback 单测 —— 2.0 阶段 1 桶 2 P1 (Tasks 62-63)
 *
 * 测试目标：
 *   - NPC_EMOJI 常量含 6 个桶 2 NPC（含 lihua）
 *   - 当 api.getTiles 返回 npc_ids 时，WorldMap 渲染 .emoji-fallback（SVG text）
 *   - emoji 字符与 NPC_EMOJI 表一致（slug → emoji）
 *
 * 注：WorldMap 不接 npcs prop，NPC 列表从 /v1/tiles 的 tile.npc_ids 派生。
 * 头像 avatar_url 1.0 不在 /v1/tiles 返回，2.0+ 才会扩展 endpoint；当前只测 emoji 路径。
 */
// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, render, waitFor } from '@testing-library/react';

vi.mock('@/lib/api', () => ({
  api: {
    getTiles: vi.fn(),
    move: vi.fn(),
    getNpc: vi.fn(),
    setToken: vi.fn(),
  },
}));

import { api } from '@/lib/api';
import { NPC_EMOJI, NPC_EMOJI_DEFAULT, WorldMap } from './WorldMap';
import { useGameStore } from '@/store/game';

const SAMPLE_TILES_WITH_NPC = [
  {
    id: 'tile_0_0',
    center_x: 50,
    center_y: 50,
    size: 100,
    lod_level: 'CBD' as const,
    buildings: [],
    npc_ids: ['npc_wang_boss_001', 'npc_book_keeper_001'],
    player_ids: ['test-player-id'],
  },
  {
    id: 'tile_1_0',
    center_x: 150,
    center_y: 50,
    size: 100,
    lod_level: 'Residential' as const,
    buildings: [],
    npc_ids: ['npc_lihua_001'],
    player_ids: [],
  },
];

describe('NPC_EMOJI 常量', () => {
  it('含 6 个桶 2 NPC（5 新 + lihua）', () => {
    expect(Object.keys(NPC_EMOJI)).toHaveLength(6);
    expect(NPC_EMOJI.npc_wang_boss_001).toBe('🍺');
    expect(NPC_EMOJI.npc_grace_healer_001).toBe('💊');
    expect(NPC_EMOJI.npc_snack_owner_001).toBe('🍢');
    expect(NPC_EMOJI.npc_book_keeper_001).toBe('📚');
    expect(NPC_EMOJI.npc_dance_leader_001).toBe('💃');
    expect(NPC_EMOJI.npc_lihua_001).toBe('👨‍💻');
  });

  it('NPC_EMOJI_DEFAULT 是通用头像字符', () => {
    expect(NPC_EMOJI_DEFAULT).toBe('👤');
  });
});

describe('WorldMap - NPC emoji fallback 渲染', () => {
  beforeEach(() => {
    window.localStorage.setItem('aicity_player_id', 'test-player-id');
    useGameStore.getState().setPlayerId('test-player-id');
    vi.mocked(api.getTiles).mockResolvedValue(SAMPLE_TILES_WITH_NPC);
    vi.mocked(api.move).mockResolvedValue({
      player_id: 'test-player-id',
      current_tile_id: 'tile_0_0',
      x: 50,
      y: 50,
      ts_ms: Date.now(),
      accepted: true,
    });
  });

  afterEach(() => {
    cleanup();
    window.localStorage.clear();
    vi.restoreAllMocks();
  });

  it('为每个 NPC 渲染 .emoji-fallback SVG text', async () => {
    const { container } = render(<WorldMap />);
    await waitFor(() => {
      const els = container.querySelectorAll('.emoji-fallback');
      if (els.length !== 3) throw new Error(`expected 3 emoji-fallback, got ${els.length}`);
    });
    const fallbacks = container.querySelectorAll('.emoji-fallback');
    expect(fallbacks).toHaveLength(3);
  });

  it('emoji 字符与 NPC_EMOJI 表一一对应', async () => {
    const { container } = render(<WorldMap />);
    await waitFor(() => {
      const els = container.querySelectorAll('.emoji-fallback');
      if (els.length !== 3) throw new Error(`expected 3 emoji-fallback, got ${els.length}`);
    });

    const wangBossText = container.querySelector('[data-npc-emoji="npc_wang_boss_001"]');
    expect(wangBossText?.textContent).toBe(NPC_EMOJI.npc_wang_boss_001);

    const bookKeeperText = container.querySelector('[data-npc-emoji="npc_book_keeper_001"]');
    expect(bookKeeperText?.textContent).toBe(NPC_EMOJI.npc_book_keeper_001);

    const lihuaText = container.querySelector('[data-npc-emoji="npc_lihua_001"]');
    expect(lihuaText?.textContent).toBe(NPC_EMOJI.npc_lihua_001);
  });

  it('NPC 圆点保留 data-npc-id 兼容现有 T03d 交互', async () => {
    const { container } = render(<WorldMap />);
    await waitFor(() => {
      const el = container.querySelector('[data-npc-id="npc_wang_boss_001"]');
      if (!el) throw new Error('npc circle not rendered');
    });
    expect(container.querySelector('[data-npc-id="npc_wang_boss_001"]')).not.toBeNull();
    expect(container.querySelector('[data-npc-id="npc_lihua_001"]')).not.toBeNull();
  });
});
