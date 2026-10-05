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
  NPC_SAY_STREAM_EVENT,
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

/**
 * 2.0 阶段 2 T19：isNpcSayStreamBeat / isNpcSayStreamDone type guard 间接测试。
 *
 * type guard 是 ws-events.ts 模块内私有函数（未 export）—— 公开契约只走
 * startWsBridge 派发的 aicity:npc_say_stream CustomEvent。本测试断言：
 *   - 合规信封 → bridge 派发 NPC_SAY_STREAM_EVENT
 *   - 缺字段 / 外层 type 错 / payload.type 错 / 字段类型错 → 不派发
 *
 * 这样既验证 type guard 行为，又不需要为了测试把内部函数 export 出去。
 */
describe('startWsBridge - npc_say_stream type guards (T19)', () => {
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

  it('test_is_npc_say_stream_beat_type_guard: accepts matching beat payload, rejects non-matching', () => {
    startWsBridge();
    const listener = onMessageSpy.mock.calls[0][0] as (msg: unknown) => void;

    // 合规 beat —— 应派发
    const validBeat = {
      type: 'npc_say_stream',
      trace_id: 't-beat-1',
      ts_ms: 1700000000000,
      payload: {
        type: 'npc_say_stream',
        npc_id: 'npc_wang_boss_001',
        session_id: 's-1',
        sentence_idx: 0,
        text: '来了您嘞！',
        emotion: 'happy',
        ts_ms: 1700000000000,
        trace_id: 't-beat-1',
      },
    };
    listener(validBeat);
    expect(dispatchEvent).toHaveBeenCalledTimes(1);
    const event = dispatchEvent.mock.calls[0][0] as CustomEvent;
    expect(event.type).toBe(NPC_SAY_STREAM_EVENT);
    expect(event.detail).toEqual(validBeat);

    // 拒绝 cases —— 应静默忽略
    const rejects: Array<{ label: string; frame: unknown }> = [
      {
        label: '外层 type 错',
        frame: { ...validBeat, type: 'npc_dialogue' },
      },
      {
        label: '内层 payload.type 错（done 落到 beat 通道）',
        frame: {
          ...validBeat,
          payload: { ...validBeat.payload, type: 'npc_say_stream_done' },
        },
      },
      {
        label: '缺 sentence_idx',
        frame: {
          ...validBeat,
          payload: {
            ...validBeat.payload,
            sentence_idx: undefined as unknown as number,
          },
        },
      },
      {
        label: 'sentence_idx 类型错（string 而非 number）',
        frame: {
          ...validBeat,
          payload: {
            ...validBeat.payload,
            sentence_idx: '0' as unknown as number,
          },
        },
      },
      {
        label: 'emotion 类型错（number 而非 string）',
        frame: {
          ...validBeat,
          payload: {
            ...validBeat.payload,
            emotion: 42 as unknown as string,
          },
        },
      },
      {
        label: 'payload 是 null',
        frame: { ...validBeat, payload: null },
      },
      {
        label: 'envelope 不是 object',
        frame: 'oops',
      },
    ];

    for (const r of rejects) {
      listener(r.frame);
    }
    // 上面只放过 1 次（validBeat），其余 7 次拒绝
    expect(dispatchEvent).toHaveBeenCalledTimes(1);
  });

  it('test_is_npc_say_stream_done_type_guard: accepts matching done payload, rejects non-matching', () => {
    startWsBridge();
    const listener = onMessageSpy.mock.calls[0][0] as (msg: unknown) => void;

    // 合规 done —— 应派发
    const validDone = {
      type: 'npc_say_stream',
      trace_id: 't-done-1',
      ts_ms: 1700000000999,
      payload: {
        type: 'npc_say_stream_done',
        npc_id: 'npc_wang_boss_001',
        session_id: 's-1',
        sentence_count: 2,
        complete: true,
        ts_ms: 1700000000999,
        trace_id: 't-done-1',
      },
    };
    listener(validDone);
    expect(dispatchEvent).toHaveBeenCalledTimes(1);
    const event = dispatchEvent.mock.calls[0][0] as CustomEvent;
    expect(event.type).toBe(NPC_SAY_STREAM_EVENT);
    expect(event.detail).toEqual(validDone);

    // 拒绝 cases —— 应静默忽略
    const rejects: Array<{ label: string; frame: unknown }> = [
      {
        label: '内层 payload.type 错（beat 落到 done 通道）',
        frame: {
          ...validDone,
          payload: { ...validDone.payload, type: 'npc_say_stream' },
        },
      },
      {
        label: 'sentence_count 缺失',
        frame: {
          ...validDone,
          payload: {
            ...validDone.payload,
            sentence_count: undefined as unknown as number,
          },
        },
      },
      {
        label: 'complete 类型错（0/1 而非 boolean）',
        frame: {
          ...validDone,
          payload: {
            ...validDone.payload,
            complete: 1 as unknown as boolean,
          },
        },
      },
      {
        label: 'session_id 缺失',
        frame: {
          ...validDone,
          payload: {
            ...validDone.payload,
            session_id: undefined as unknown as string,
          },
        },
      },
      {
        label: 'payload 是 primitive',
        frame: { ...validDone, payload: 'done' },
      },
    ];

    for (const r of rejects) {
      listener(r.frame);
    }
    expect(dispatchEvent).toHaveBeenCalledTimes(1);
  });
});