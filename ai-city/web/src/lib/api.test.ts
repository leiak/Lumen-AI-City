/**
 * api client 单元测试 —— Sprint 12 T03b。
 *
 * 测试目标：`api.postNpcTalk(npcId, choiceId, playerId)`
 *   - POST /v1/npc/talk
 *   - body = {npc_id, player_id, choice_id}
 *   - Authorization: Bearer <token>（token 来自 api.setToken）
 *   - 非 2xx 抛错
 *
 * mock 策略（vitest node env）：
 *   - vi.stubGlobal('fetch', fetchMock) 拦截底层 fetch
 *   - api.setToken('test-jwt') 直接走 ApiClient.setToken（与登录态同款）
 *   - 不动 ws-events.ts / zustand store（postNpcTalk 是同步 HTTP 客户端方法）
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from './api';

const NPC_DIALOGUE_RESPONSE = {
  npc_id: 'npc_wang_boss_001',
  player_id: 'player-1',
  tile_id: 'tile_0_0',
  say: '来我店看看？',
  options: [
    { id: 'yes', text: '好' },
    { id: 'no', text: '不' },
  ],
  reply_to_choice_id: 'ask_business',
};

describe('api.postNpcTalk (T03b)', () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);
    api.setToken('test-jwt');
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    api.setToken(''); // 清 token，避免跨 test 串
  });

  it('POSTs to /v1/npc/talk with Bearer auth and JSON body', async () => {
    fetchMock.mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => NPC_DIALOGUE_RESPONSE,
    });

    const result = await api.postNpcTalk(
      'npc_wang_boss_001',
      'ask_business',
      'player-1',
    );

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe('http://localhost:8080/v1/npc/talk');
    expect(init.method).toBe('POST');
    expect(init.headers['Content-Type']).toBe('application/json');
    expect(init.headers.Authorization).toBe('Bearer test-jwt');
    expect(JSON.parse(init.body)).toEqual({
      npc_id: 'npc_wang_boss_001',
      player_id: 'player-1',
      choice_id: 'ask_business',
    });

    // 返回值形状与 ws-gateway npc_dialogue 信封 payload 一致
    expect(result.say).toBe('来我店看看？');
    expect(result.options).toHaveLength(2);
    expect(result.options[0].id).toBe('yes');
    expect(result.reply_to_choice_id).toBe('ask_business');
    expect(result.npc_id).toBe('npc_wang_boss_001');
  });

  it('throws on non-2xx response with status in message', async () => {
    fetchMock.mockResolvedValue({
      ok: false,
      status: 404,
      text: async () => 'npc not found',
    });

    await expect(
      api.postNpcTalk('npc_ghost', 'ask', 'player-1'),
    ).rejects.toThrow(/404/);
  });

  it('throws when no token is set (request without Authorization header)', async () => {
    // 重置 token，模拟未登录态
    api.setToken('');
    fetchMock.mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => NPC_DIALOGUE_RESPONSE,
    });

    await api.postNpcTalk('npc_wang_boss_001', 'ask', 'player-1');

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [, init] = fetchMock.mock.calls[0];
    // 与现有 ApiClient.request 一致：无 token 时不发 Authorization header
    expect(init.headers.Authorization).toBeUndefined();
  });
});