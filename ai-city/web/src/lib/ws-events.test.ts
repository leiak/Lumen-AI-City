/**
 * ws-events bridge 单元测试 —— Sprint 12 T03a。
 *
 * 测试目标：`startWsBridge` 把 ws-gateway 的 npc_dialogue 信封转成分发到
 * window 的 `aicity:npc_dialogue` CustomEvent，detail 用完整信封。
 *
 * mock 策略（vitest node env，没有 DOM）：
 *   - MockWS 替身让 ws.connect() 不报 WebSocket undefined
 *   - vi.stubGlobal('window', {...}) 给 startWsBridge 提供 localStorage
 *     与 dispatchEvent（避免 typeof window === 'undefined' 早退）
 *   - vi.spyOn(ws, 'onMessage') 截获注册到 singleton 的回调，直接调它
 *     注入 WS frame —— 不走真实 WebSocket，绕开 JSON 序列化往返
 *
 * ws.ts 是 singleton（listeners Set 跨调用累积），每个 test 都新建 spy，
 * 不依赖内部状态；afterEach 用 vi.restoreAllMocks + vi.unstubAllGlobals 清理。
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  startWsBridge,
  NPC_DIALOGUE_EVENT,
  PLAYER_MOVED_EVENT,
} from './ws-events';
import { ws } from './ws';

class MockWS {
  static CONNECTING = 0;
  static OPEN = 1;
  static CLOSING = 2;
  static CLOSED = 3;
  onopen: ((ev: Event) => void) | null = null;
  onmessage: ((ev: MessageEvent) => void) | null = null;
  onclose: ((ev: CloseEvent) => void) | null = null;
  onerror: ((ev: Event) => void) | null = null;
  readyState = 0;
  close(): void {}
  send(): void {}
}

describe('startWsBridge - npc_dialogue (T03a)', () => {
  // onMessageSpy 类型由 vi.spyOn 推断；cast 到 any 以兼容 strict 模式下
  // vi.spyOn 的复杂泛型签名。运行时行为由 spy.mock.calls / toHaveBeenCalledTimes 保证。
  let onMessageSpy: any;
  let dispatchEvent: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    vi.stubGlobal('WebSocket', MockWS);
    dispatchEvent = vi.fn();
    vi.stubGlobal('window', {
      localStorage: {
        getItem: vi.fn((k: string) => (k === 'aicity_token' ? 'test-tkn' : null)),
        setItem: vi.fn(),
        removeItem: vi.fn(),
      },
      dispatchEvent,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    });
    onMessageSpy = vi.spyOn(ws, 'onMessage');
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it('dispatches aicity:npc_dialogue CustomEvent on npc_dialogue frame (active say)', () => {
    startWsBridge();
    expect(onMessageSpy).toHaveBeenCalledTimes(1);
    const listener = onMessageSpy.mock.calls[0][0] as (msg: unknown) => void;

    const frame = {
      type: 'npc_dialogue',
      trace_id: 't-active-1',
      ts_ms: 1700000000000,
      payload: {
        npc_id: 'npc_wang_boss_001',
        player_id: '',
        tile_id: '',
        say: '来了您嘞！',
        options: [],
        reply_to_choice_id: null,
      },
    };

    listener(frame);

    expect(dispatchEvent).toHaveBeenCalledTimes(1);
    const event = dispatchEvent.mock.calls[0][0] as CustomEvent;
    expect(event.type).toBe(NPC_DIALOGUE_EVENT);
    // detail 是完整信封：消费方按 e.detail.payload.{npc_id,say,...} 取值
    expect(event.detail).toEqual(frame);
    expect(event.detail.payload.npc_id).toBe('npc_wang_boss_001');
    expect(event.detail.payload.say).toBe('来了您嘞！');
    expect(event.detail.payload.reply_to_choice_id).toBeNull();
    expect(event.detail.trace_id).toBe('t-active-1');
  });

  it('handles npc_dialogue reply (reply_to_choice_id non-null + non-empty player/tile)', () => {
    startWsBridge();
    const listener = onMessageSpy.mock.calls[0][0] as (msg: unknown) => void;

    const frame = {
      type: 'npc_dialogue',
      trace_id: 't-reply-1',
      ts_ms: 1700000000001,
      payload: {
        npc_id: 'npc_wang_boss_001',
        player_id: 'player-1',
        tile_id: 'tile_0_0',
        say: '回您一句。',
        options: [{ id: 'ok', text: '好的' }, { id: 'bye', text: '再见' }],
        reply_to_choice_id: 'ask_business',
      },
    };

    listener(frame);

    expect(dispatchEvent).toHaveBeenCalledTimes(1);
    const event = dispatchEvent.mock.calls[0][0] as CustomEvent;
    expect(event.type).toBe(NPC_DIALOGUE_EVENT);
    expect(event.detail.payload.reply_to_choice_id).toBe('ask_business');
    expect(event.detail.payload.player_id).toBe('player-1');
    expect(event.detail.payload.tile_id).toBe('tile_0_0');
    expect(event.detail.payload.options).toHaveLength(2);
    expect(event.detail.payload.options[0].id).toBe('ok');
    expect(event.detail.payload.options[1].text).toBe('再见');
  });

  it('player_moved still works (regression)', () => {
    startWsBridge();
    const listener = onMessageSpy.mock.calls[0][0] as (msg: unknown) => void;

    listener({
      type: 'player_moved',
      trace_id: 't-move-1',
      ts_ms: 1700000000002,
      payload: {
        player_id: 'p-1',
        tile_id: 'tile_0_0',
        x: 50,
        y: 50,
        ts_ms: 1700000000002,
      },
    });

    expect(dispatchEvent).toHaveBeenCalledTimes(1);
    const event = dispatchEvent.mock.calls[0][0] as CustomEvent;
    expect(event.type).toBe(PLAYER_MOVED_EVENT);
    // player_moved 仍走老 detail 形状（payload 直接作为 detail，便于
    // WorldMap.onPlayerMoved 直接读 player_id / x / y）
    expect(event.detail.player_id).toBe('p-1');
    expect(event.detail.tile_id).toBe('tile_0_0');
  });

  it('does not dispatch for unknown message type', () => {
    startWsBridge();
    const listener = onMessageSpy.mock.calls[0][0] as (msg: unknown) => void;

    listener({ type: 'some_other_event', payload: { foo: 'bar' } });

    expect(dispatchEvent).not.toHaveBeenCalled();
  });

  it('does not dispatch for malformed npc_dialogue (missing say)', () => {
    startWsBridge();
    const listener = onMessageSpy.mock.calls[0][0] as (msg: unknown) => void;

    listener({
      type: 'npc_dialogue',
      trace_id: 't-bad',
      ts_ms: 1,
      payload: { npc_id: 'x' }, // 缺 say —— type guard 应拒绝
    });

    expect(dispatchEvent).not.toHaveBeenCalled();
  });
});