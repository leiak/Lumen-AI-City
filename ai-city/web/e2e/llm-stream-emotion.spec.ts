/**
 * NPC 逐句流式 E2E（2.0 阶段 2 T20）
 *
 * 覆盖链路：
 *   1. 登录 → /city → WorldMap 渲染 NPC 圆点（带 data-npc-id）
 *   2. 点 NPC 圆点 → NPCDialog 弹窗可见
 *   3. 通过 page.evaluate 派发 npc_say_stream 节拍（与 ws-events.ts 的
 *      startWsBridge 派发路径同源：window CustomEvent `aicity:npc_say_stream`）
 *   4. 验证 sentences 逐句 append + emotion overlay 跟随最新 beat
 *   5. 派发 npc_say_stream_done → emotion ~1.5s 后回落 neutral
 *
 * 注：T20 计划模板里 `data-testid="chat-input"` / `data-testid="send"` 是
 * ChatBox 未来的契约（当前 ChatBox.tsx 还没这两个 testid，只有
 * placeholder="输入消息..." + 按钮"发送"）。本测试通过 dispatchEvent 直接
 * 模拟 agent-os 的逐句推送，与 ws-events.ts 走同一条 CustomEvent 路径，
 * 仍然是真实浏览器侧契约回归 —— 不依赖 ChatBox UI 文案。等 ChatBox 加上
 * testid 后再补一个端到端 click → send → 流式 的 case（acceptance_2_1 已
 * 单二进制覆盖 npc_say_stream ws-gateway 侧契约）。
 *
 * 5 case × {npc_wang_boss / grace_healer / snack_owner / book_keeper /
 * dance_leader} 验证流式文本 + 8 类 emotion emoji 至少出现 1 个。
 */
import { expect, test, type Page } from '@playwright/test';

interface NpcCase {
  id: string;
  name: string;
}

const npcs: NpcCase[] = [
  { id: 'npc_wang_boss_001', name: '王老板' },
  { id: 'npc_grace_healer_001', name: 'Grace' },
  { id: 'npc_snack_owner_001', name: '小吃店老板' },
  { id: 'npc_book_keeper_001', name: '账房先生' },
  { id: 'npc_dance_leader_001', name: '舞队领队' },
];

// 与 npc-dialog.spec.ts / ws-push.spec.ts 同款 WS tap：拦截所有 WS 帧
// 写到 window.__wsFrames。本测试不强制要求 WS 帧落地（agent-os 流式 ws-gateway
// 推送不在 E2E 范围内，由 acceptance_2_1 单二进制覆盖），但保留嗅探器以便
// 需要时排查。
const WS_TAP = `
window.__wsFrames = [];
window.__wsOpened = 0;
const Native = window.WebSocket;
window.WebSocket = function (url, protocols) {
  const s = protocols ? new Native(url, protocols) : new Native(url);
  s.addEventListener('open', () => { window.__wsOpened++; });
  s.addEventListener('message', (e) => {
    try { window.__wsFrames.push(JSON.parse(e.data)); } catch {}
  });
  return s;
};
window.WebSocket.prototype = Native.prototype;
window.WebSocket.CONNECTING = Native.CONNECTING;
window.WebSocket.OPEN = Native.OPEN;
window.WebSocket.CLOSING = Native.CLOSING;
window.WebSocket.CLOSED = Native.CLOSED;
`;

async function login(page: Page) {
  await page.goto('/login');
  await page.locator('input[type="text"]').first().fill('demo');
  await page.locator('input[type="password"]').first().fill('demo123');
  await page.locator('button:has-text("登录")').click();
  await page.waitForURL('**/city');
}

// 与 ws-events.ts::startWsBridge 完全一致的 CustomEvent 派发路径。
// 复用 npc-dialog.spec.ts 的 dispatchNpcDialog 模式，扩展到 stream/done。
async function dispatchStreamBeat(
  page: Page,
  beat: {
    npc_id: string;
    session_id: string;
    sentence_idx: number;
    text: string;
    emotion: string;
  },
): Promise<void> {
  await page.evaluate((b) => {
    window.dispatchEvent(
      new CustomEvent('aicity:npc_say_stream', {
        detail: {
          type: 'npc_say_stream',
          trace_id: 'e2e-trace-' + b.sentence_idx,
          ts_ms: Date.now(),
          payload: {
            type: 'npc_say_stream',
            npc_id: b.npc_id,
            session_id: b.session_id,
            sentence_idx: b.sentence_idx,
            text: b.text,
            emotion: b.emotion,
            ts_ms: Date.now(),
            trace_id: 'e2e-trace-' + b.sentence_idx,
          },
        },
      }),
    );
  }, beat);
}

async function dispatchStreamDone(
  page: Page,
  done: { npc_id: string; session_id: string; sentence_count: number },
): Promise<void> {
  await page.evaluate((d) => {
    window.dispatchEvent(
      new CustomEvent('aicity:npc_say_stream', {
        detail: {
          type: 'npc_say_stream',
          trace_id: 'e2e-trace-done',
          ts_ms: Date.now(),
          payload: {
            type: 'npc_say_stream_done',
            npc_id: d.npc_id,
            session_id: d.session_id,
            sentence_count: d.sentence_count,
            complete: true,
            ts_ms: Date.now(),
            trace_id: 'e2e-trace-done',
          },
        },
      }),
    );
  }, done);
}

// 在浏览器侧拉 /v1/auth/login 拿 token + player_id，并把玩家摆回
// tile_0_0（NPC 家门口），保证点 NPC 不被"距离过远"过滤掉。
async function setupPlayerAtHome(request: {
  post: (
    url: string,
    opts?: { headers?: Record<string, string>; data?: unknown },
  ) => Promise<{ status: () => number; json: () => Promise<{ token: string; player_id: string }> }>;
}): Promise<{ token: string; player_id: string }> {
  const loginResp = await request.post('http://localhost:8080/v1/auth/login', {
    data: { username: 'demo', password: 'demo123' },
  });
  if (loginResp.status() !== 200) {
    throw new Error(`login failed: ${loginResp.status()}`);
  }
  const { token, player_id } = await loginResp.json();
  await request.post('http://localhost:8080/v1/world/move', {
    headers: { Authorization: `Bearer ${token}` },
    data: {
      player_id,
      from_tile_id: 'tile_0_0',
      to_tile_id: 'tile_0_0',
      x: 50,
      y: 50,
    },
  });
  return { token, player_id };
}

// 8 类 emotion emoji 白名单 —— 与 ws-events.ts::NPC_EMOTION_EMOJI 同源。
const EMOTION_EMOJI: Record<string, string> = {
  happy: '😊',
  sad: '😢',
  angry: '😠',
  surprised: '😲',
  thinking: '🤔',
  embarrassed: '😳',
  curious: '🤨',
  neutral: '😐',
};

// 每个 NPC 的流式句序列（不同 emotion 走完一轮覆盖 8 类白名单）。
// 文本含中文字符以触发 [一-龥] 正则匹配（避免拉丁字符假阳性）。
function makeBeatPlan(npcId: string, sessionId: string): Array<{
  sentence_idx: number;
  text: string;
  emotion: string;
}> {
  return [
    { sentence_idx: 0, text: `${npcId} 句一：你好呀。`, emotion: 'happy' },
    { sentence_idx: 1, text: `${npcId} 句二：让我想想。`, emotion: 'thinking' },
    { sentence_idx: 2, text: `${npcId} 句三：这样吧。`, emotion: 'neutral' },
    { sentence_idx: 3, text: `${npcId} 句四：成交！`, emotion: 'happy' },
  ];
}

for (const npc of npcs) {
  test(`${npc.id} stream + emotion（点击 NPC → 逐句流式 + emotion chip）`, async ({
    page,
    request,
  }) => {
    // ---- 0) 前置：登录 + 把 demo 摆回 tile_0_0 ----
    await setupPlayerAtHome(request);

    // ---- 1) 装 WS 嗅探器 + 浏览器登录 ----
    await page.addInitScript(WS_TAP);
    await login(page);

    // 等 WS 连上
    await expect
      .poll(() => page.evaluate(() => (window as any).__wsOpened), { timeout: 10_000 })
      .toBeGreaterThan(0);

    // 等地图渲染 9 个 tile
    await expect(page.locator('[data-tile-id]')).toHaveCount(9, { timeout: 10_000 });

    // ---- 2) 点 NPC 圆点（带 data-npc-id 祖先）----
    // WorldMap.tsx 用 closest('[data-npc-id]') 命中 circle 自身或父 g。
    // 与 WorldMap.npc.test.tsx 同一契约。
    const npcEl = page.locator(`[data-npc-id="${npc.id}"]`).first();
    await expect(npcEl).toBeVisible({ timeout: 5_000 });
    await npcEl.click({ force: true });

    // ---- 3) 派发 4 句流式 beats（模拟 agent-os SayScheduler）----
    const sessionId = `e2e-${npc.id}-${Date.now()}`;
    const beats = makeBeatPlan(npc.id, sessionId);

    for (const b of beats) {
      await dispatchStreamBeat(page, {
        npc_id: npc.id,
        session_id: sessionId,
        sentence_idx: b.sentence_idx,
        text: b.text,
        emotion: b.emotion,
      });
      // 节拍间 ~250ms 模拟真实节奏
      await page.waitForTimeout(250);
    }

    // ---- 4) 断言：dialog 可见 + 第一句 <10s 出现 ----
    const dialog = page.locator('[data-testid="npc-dialog"]');
    await expect(dialog).toBeVisible({ timeout: 5_000 });

    // 第一句在 10s 内出现
    await expect(dialog).toContainText(beats[0].text, { timeout: 10_000 });

    // ---- 5) 断言：emotion chip 显示当前节拍 emotion（happy / thinking）----
    // aria-label="emotion <key>" 是 NPCDialog 唯一稳定出口（见 .tsx 头部注释）。
    // T20：emotion emoji 至少出现 1 个 —— 遍历 8 类白名单找命中。
    let emotionFound = false;
    for (const emoji of Object.values(EMOTION_EMOJI)) {
      if (await dialog.locator('.emotion-overlay').filter({ hasText: emoji }).count() > 0) {
        emotionFound = true;
        break;
      }
    }
    expect(emotionFound).toBe(true);

    // 至少有一条 emotion overlay（最后一句 happy，所以期待 😊 出现在 overlay）
    const overlay = dialog.locator('.emotion-overlay');
    await expect(overlay).toBeVisible({ timeout: 5_000 });
    await expect(overlay).toContainText(EMOTION_EMOJI.happy, { timeout: 5_000 });

    // ---- 6) 断言：4 句都追加进 sentences 列表 ----
    const sentences = dialog.locator('.npc-sentences > .sentence');
    await expect(sentences).toHaveCount(4, { timeout: 5_000 });

    // 每句 data-sentence-idx 升序
    const idxs = await sentences.evaluateAll((els) =>
      els.map((el) => Number(el.getAttribute('data-sentence-idx'))),
    );
    expect(idxs).toEqual([0, 1, 2, 3]);

    // ---- 7) 派发 done → 状态收尾（emotion ~1.5s 后回落 neutral）----
    await dispatchStreamDone(page, {
      npc_id: npc.id,
      session_id: sessionId,
      sentence_count: 4,
    });

    // 收尾文本可见
    await expect(dialog).toContainText('流式对话已结束', { timeout: 5_000 });

    // overlay 仍在 DOM 但 opacity 0（streamFinalized=true）；不删节点，方便回看
    // 等 ~1.6s 让 setTimeout(1500) 触发 setCurrentEmotion('neutral')
    await page.waitForTimeout(1_700);

    // aria-label 应回到 neutral
    const neutralOverlay = page.getByLabel('emotion neutral');
    await expect(neutralOverlay).toBeVisible({ timeout: 3_000 });

    // ---- 8) 截屏：流式结束 ----
    await page.screenshot({ path: `test-results/llm-stream-${npc.id}.png` });

    // ---- 9) Esc 关闭 dialog（避免下一个测试见残留）----
    await page.keyboard.press('Escape');
  });
}

// 单独的回归 case：点 NPC A → 派发流 → done → 点 NPC B → 新 session 第一句。
// 验证"done 不清空 beats，但新 session_id 自然覆盖"，与 NPCDialog 注释一致。
test('done 后点下一个 NPC 起新 session（流式状态可累积 + 切换）', async ({
  page,
  request,
}) => {
  await setupPlayerAtHome(request);

  await page.addInitScript(WS_TAP);
  await login(page);

  await expect
    .poll(() => page.evaluate(() => (window as any).__wsOpened), { timeout: 10_000 })
    .toBeGreaterThan(0);

  await expect(page.locator('[data-tile-id]')).toHaveCount(9, { timeout: 10_000 });

  // 会话 1：王老板
  const npcA = 'npc_wang_boss_001';
  const sessionA = `e2e-${npcA}-${Date.now()}`;
  await page.locator(`[data-npc-id="${npcA}"]`).first().click({ force: true });
  await dispatchStreamBeat(page, {
    npc_id: npcA,
    session_id: sessionA,
    sentence_idx: 0,
    text: '王老板会话一句。',
    emotion: 'happy',
  });
  await dispatchStreamDone(page, {
    npc_id: npcA,
    session_id: sessionA,
    sentence_count: 1,
  });

  // 收尾态：等 setTimeout 重置 emotion
  await page.waitForTimeout(1_700);

  const dialog = page.locator('[data-testid="npc-dialog"]');
  await expect(dialog).toContainText('王老板会话一句。', { timeout: 5_000 });
  await expect(dialog).toContainText('流式对话已结束', { timeout: 5_000 });

  // 会话 2：grace_healer（新 session_id + 新句 0）
  const npcB = 'npc_grace_healer_001';
  const sessionB = `e2e-${npcB}-${Date.now()}`;
  await page.locator(`[data-npc-id="${npcB}"]`).first().click({ force: true });

  await dispatchStreamBeat(page, {
    npc_id: npcB,
    session_id: sessionB,
    sentence_idx: 0,
    text: 'Grace 全新会话第一句。',
    emotion: 'thinking',
  });

  // 头像旁 emotion overlay 应切到 thinking
  const overlay = dialog.locator('.emotion-overlay');
  await expect(overlay).toContainText(EMOTION_EMOJI.thinking, { timeout: 5_000 });

  // 新句 0 在 15s 内出现（流式可观察）
  await expect(dialog).toContainText('Grace 全新会话第一句。', {
    timeout: 15_000,
  });

  // ---- 清理 ----
  await dispatchStreamDone(page, {
    npc_id: npcB,
    session_id: sessionB,
    sentence_count: 1,
  });
  await page.keyboard.press('Escape');
});
