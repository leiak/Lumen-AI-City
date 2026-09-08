/**
 * CityPage 集成单测 —— Sprint 12 T03e。
 *
 * 测试目标：
 *   - CityPage 在顶层挂载 <NPCDialog />（T03a-c 引入）
 *   - 派发 aicity:npc_dialogue CustomEvent 时，dialog 出现并显示 say 文本
 *
 * mock 策略（vitest jsdom env）：
 *   - vi.mock('@/lib/api', factory) 替身 api.getTiles，避免 WorldMap mount 时
 *     真实拉 /v1/tiles 失败
 *   - 不设 aicity_token：startWsBridge() 在 token 缺失时打 console.warn
 *     然后返回 no-op cleanup（不会真连 WS），所以测试无需 mock ws.ts
 *   - fireEvent(window, new CustomEvent(...)) 后用 findByText 断言 —— T03c
 *     单测中已验证同一 CustomEvent 路径在 NPCDialog 内渲染 payload.say
 */
// @vitest-environment jsdom
import { afterEach, beforeEach, describe, it, expect, vi } from 'vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';

vi.mock('@/lib/api', () => ({
  api: {
    getTiles: vi.fn().mockResolvedValue([]),
    move: vi.fn(),
    setToken: vi.fn(),
    postNpcTalk: vi.fn(),
  },
}));

import CityPage from './page';

describe('CityPage (T03e)', () => {
  afterEach(() => {
    cleanup();
  });

  beforeEach(() => {
    // 让 useGameStore 初始 state 在测试间隔离 —— playerId/position 重置
    // （zustand store 跨测试持久，否则上一个 test 的位置会污染）
    window.localStorage.clear();
  });

  it('mounts NPCDialog so aicity:npc_dialogue events render the dialog', async () => {
    render(<CityPage />);

    // 没有事件时 dialog 不应出现（NPCDialog 内部 payload === null → return null）
    expect(screen.queryByRole('dialog')).toBeNull();

    // 派发 npc_dialogue 信封：detail.payload.say === 'T03e smoke'
    fireEvent(
      window,
      new CustomEvent('aicity:npc_dialogue', {
        detail: {
          type: 'npc_dialogue',
          trace_id: 't-t03e',
          ts_ms: Date.now(),
          payload: {
            npc_id: 'npc_wang_boss_001',
            player_id: '',
            tile_id: '',
            say: 'T03e smoke',
            options: [{ id: 'opt-1', text: '选项 1' }],
            reply_to_choice_id: null,
          },
        },
      }),
    );

    // 关键断言：dialog 出现并显示 payload.say —— 证明 CityPage 真的挂了 <NPCDialog />
    expect(await screen.findByText('T03e smoke')).toBeInTheDocument();
    expect(screen.getByRole('dialog')).toBeInTheDocument();
  });
});
