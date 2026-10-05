/**
 * NPCDialog 逐句流式渲染单测 —— 2.0 阶段 2 T18+T19。
 *
 * 测试目标（sentence-by-sentence render behavior added in T18）：
 *   - 初始态：没有任何节拍时 NPCDialog 不渲染
 *   - 节拍累加：aicity:npc_say_stream 节拍包 → 渲染对应 <p className="sentence">
 *   - emotion overlay：头像旁显示当前节拍的 emotion emoji（aria-label="emotion <key>"）
 *   - 8 类 emotion 全部白名单命中
 *   - (session_id, sentence_idx) 复合键去重 —— 重发同 idx 不产生重复行
 *   - 结束标记：aicity:npc_say_stream_done → ~1.5s 后 emotion 还原 neutral
 *   - 防御性兜底：未知 emotion 回退 neutral；malformed envelope 不崩溃
 *
 * mock 策略（vitest jsdom env）：
 *   - vi.mock('@/lib/api', factory) 替身 api.postNpcTalk —— 防 NPCDialog
 *     mount 时调网络
 *   - 直接 fireEvent(window, new CustomEvent(...)) 派发 aicity:npc_say_stream
 *     信封；不依赖 startWsBridge 内部 type guard（详见 ws-events.test.ts）
 *   - vi.useFakeTimers + vi.advanceTimersByTime —— 消除 done 后 1500ms
 *     resetEmotion 的 timing flake
 */
// @vitest-environment jsdom
import {
  afterEach,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from 'vitest';
import { cleanup, fireEvent, render, screen, act } from '@testing-library/react';

vi.mock('@/lib/api', () => ({
  api: {
    postNpcTalk: vi.fn(),
  },
}));

import { NPCDialog } from './NPCDialog';
import type { NpcSayStreamBeat, NpcSayStreamDone } from './NPCDialog.types';
import { NPC_EMOTION_EMOJI, NPC_EMOTIONS } from '@/lib/ws-events';

// --- helpers -----------------------------------------------------------

function fireStreamBeat(beat: NpcSayStreamBeat) {
  fireEvent(
    window,
    new CustomEvent('aicity:npc_say_stream', {
      detail: {
        type: 'npc_say_stream',
        trace_id: beat.trace_id,
        ts_ms: beat.ts_ms,
        payload: beat,
      },
    }),
  );
}

function fireStreamDone(done: NpcSayStreamDone) {
  fireEvent(
    window,
    new CustomEvent('aicity:npc_say_stream', {
      detail: {
        type: 'npc_say_stream',
        trace_id: done.trace_id,
        ts_ms: done.ts_ms,
        payload: done,
      },
    }),
  );
}

function makeBeat(
  partial: Partial<NpcSayStreamBeat> & {
    sentence_idx: number;
    text: string;
    emotion: string;
  },
): NpcSayStreamBeat {
  return {
    type: 'npc_say_stream',
    npc_id: 'npc_wang_boss_001',
    session_id: 's-1',
    sentence_idx: partial.sentence_idx,
    text: partial.text,
    emotion: partial.emotion,
    ts_ms: 1700000000000 + partial.sentence_idx,
    trace_id: `t-${partial.sentence_idx}`,
    ...partial,
  };
}

function makeDone(
  partial: Partial<NpcSayStreamDone> = {},
): NpcSayStreamDone {
  return {
    type: 'npc_say_stream_done',
    npc_id: 'npc_wang_boss_001',
    session_id: 's-1',
    sentence_count: 2,
    complete: true,
    ts_ms: 1700000000999,
    trace_id: 't-done',
    ...partial,
  };
}

// --- tests -------------------------------------------------------------

describe('NPCDialog sentence-by-sentence (T18+T19)', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });
  afterEach(() => {
    cleanup();
    vi.useRealTimers();
  });

  // 1. 初始态：没有 payload + 没有 beats → 不渲染
  it('test_npc_dialog_renders_initial_state: no render when no beats or payload', () => {
    render(<NPCDialog />);
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(screen.queryByText('😐')).toBeNull();
  });

  // 2. dispatch 一个 npc_say_stream beat → 渲染对应句子
  it('test_npc_dialog_appends_sentence_on_beat: dispatch beat → sentence appears', () => {
    render(<NPCDialog />);
    fireStreamBeat(
      makeBeat({ sentence_idx: 0, text: '来了您嘞！', emotion: 'happy' }),
    );

    expect(screen.getByText('来了您嘞！')).toBeInTheDocument();
    // data-sentence-idx 在流式句子上由 NPCDialog 写入
    expect(
      document.querySelector('[data-sentence-idx="0"]'),
    ).toBeInTheDocument();
  });

  // 3. emotion chip：头像旁 emotion overlay 渲染最新 emotion 的 emoji
  it('test_npc_dialog_shows_emotion_chip: avatar overlay shows active emotion emoji', () => {
    render(<NPCDialog />);
    fireStreamBeat(
      makeBeat({ sentence_idx: 0, text: '嗯？', emotion: 'thinking' }),
    );

    // overlay 的 emoji 通过 aria-label="emotion thinking" 暴露给读屏
    const chip = screen.getByLabelText('emotion thinking');
    expect(chip).toBeInTheDocument();
    expect(chip).toHaveTextContent(NPC_EMOTION_EMOJI.thinking);
  });

  // 4. 8 类 emotion 全白名单命中
  it('test_npc_dialog_handles_8_emotions: each allowed emotion renders its emoji', () => {
    render(<NPCDialog />);

    for (let i = 0; i < NPC_EMOTIONS.length; i++) {
      const emotion = NPC_EMOTIONS[i];
      fireStreamBeat(
        makeBeat({
          session_id: 's-1',
          sentence_idx: i,
          text: `say-${emotion}`,
          emotion,
        }),
      );
      const chip = screen.getByLabelText(`emotion ${emotion}`);
      expect(chip).toHaveTextContent(NPC_EMOTION_EMOJI[emotion]);
    }
    // 8 句全部渲染
    expect(screen.getByText('say-happy')).toBeInTheDocument();
    expect(screen.getByText('say-neutral')).toBeInTheDocument();
  });

  // 5. (session_id, sentence_idx) 复合键去重 —— 同 idx 重发不产生重复行
  it('test_npc_dialog_dedup_replay: same sentence_idx twice → only one row', () => {
    render(<NPCDialog />);
    const beat = makeBeat({
      sentence_idx: 0,
      text: '重复句',
      emotion: 'neutral',
    });

    fireStreamBeat(beat);
    fireStreamBeat(beat); // 重连补帧：同 idx 再发一次

    // 文本只出现一次；data-sentence-idx="0" 也只有一个 DOM 节点
    expect(screen.getAllByText('重复句')).toHaveLength(1);
    expect(
      document.querySelectorAll('[data-sentence-idx="0"]'),
    ).toHaveLength(1);
  });

  // 6. done 事件：~1.5s 后 emotion 还原 neutral（chip 淡出窗口）
  it('test_npc_dialog_done_event: done resets emotion to neutral after 1.5s', () => {
    render(<NPCDialog />);
    fireStreamBeat(
      makeBeat({ sentence_idx: 0, text: '好嘞。', emotion: 'happy' }),
    );
    expect(screen.getByLabelText('emotion happy')).toBeInTheDocument();

    fireStreamDone(makeDone());

    // 1500ms 内 emotion 仍是 happy（chip 处于淡出动画窗口）
    act(() => {
      vi.advanceTimersByTime(1499);
    });
    expect(screen.getByLabelText('emotion happy')).toBeInTheDocument();

    // 跨过 1500ms → emotion 还原 neutral（setTimeout 触发 setCurrentEmotion('neutral')）
    act(() => {
      vi.advanceTimersByTime(1);
    });
    expect(screen.getByLabelText('emotion neutral')).toBeInTheDocument();
  });

  // 7. malformed envelope（detail.payload 非对象 / 缺字段）→ 不抛错
  it('test_npc_dialog_malformed_event_ignored: bad payload does not crash', () => {
    render(<NPCDialog />);

    // 信封 type 不对（防 ws-gateway 透传错包）
    fireEvent(
      window,
      new CustomEvent('aicity:npc_say_stream', {
        detail: { type: 'npc_say_stream', payload: null },
      }),
    );
    // payload 不是对象
    fireEvent(
      window,
      new CustomEvent('aicity:npc_say_stream', {
        detail: {
          type: 'npc_say_stream',
          payload: { type: 'npc_say_stream', npc_id: 123 }, // 字段类型错
        },
      }),
    );
    // payload 是 primitive
    fireEvent(
      window,
      new CustomEvent('aicity:npc_say_stream', {
        detail: { type: 'npc_say_stream', payload: 'oops' },
      }),
    );

    // 不崩溃且不渲染任何句子
    expect(screen.queryByRole('dialog')).toBeNull();
  });

  // 8. unknown emotion → 兜底回 neutral（emoji 😐）
  it('test_npc_dialog_unknown_emotion_falls_back_to_neutral: unknown emotion renders neutral emoji', () => {
    render(<NPCDialog />);
    fireStreamBeat(
      makeBeat({
        sentence_idx: 0,
        text: '??',
        emotion: 'totally-not-an-emotion',
      }),
    );

    // aria-label 仍暴露 raw emotion（前端不做改写，便于排查 wire contract）
    // 但 emoji 渲染必须是 neutral 兜底
    const chip = screen.getByLabelText(
      'emotion totally-not-an-emotion',
    );
    expect(chip).toHaveTextContent(NPC_EMOTION_EMOJI.neutral);
    // 句子侧 inline emoji 也走 emojiForEmotion 兜底
    expect(
      screen.getByText('??').textContent,
    ).toContain(NPC_EMOTION_EMOJI.neutral);
  });
});
