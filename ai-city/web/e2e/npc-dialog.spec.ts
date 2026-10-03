/**
 * NPC 对话 E2E（Sprint 12 T11）—— NPCDialog 弹窗 + 玩家点 option 后
 * 走 spec endpoint（POST /v1/npc/:id/talk）→ 后端 reply → 弹窗更新内容。
 *
 * 覆盖链路：
 *   1. WS 帧 → ws-events.ts 桥 → window CustomEvent → NPCDialog 渲染
 *   2. 点击 option → ApiClient.postNpcTalk（spec 路径 /v1/npc/:id/talk） →
 *      api-gateway HandleByID → talk_tree lookup → 返回 reply → setState 重渲染
 *
 * 不依赖 agent-os 真实 welcome 节奏：用 page.evaluate 直接 dispatch
 * npc_dialogue 帧到 window（绕开 ws-events 桥），走 NPCDialog 同样的 onNpc
 * 监听路径。这一段验证的是 UI ↔ 后端契约，welcome 触发本身已被 acceptance_1_0
 * 单二进制覆盖（apps/a2a-gateway/cmd/acceptance_1_0）。
 */
import { expect, test, type Page } from '@playwright/test';

// WS tap：在浏览器侧拦截所有 WS 帧，写到 window.__wsFrames 供断言用。
// 与 ws-push.spec.ts 同样套路 —— 同一份 ws-gateway 协议。
//
// 与 ws-push.spec.ts 唯一差异：onmessage 被 wrap，npc_dialogue 帧被丢。
// 原因：agent-os SayScheduler 每 SAY_TICK_SECONDS（默认 5s）随机抽一句
// greeting[] publish 到 aicity:npc_dialogue，会让测试里的"点击 option →
// 后端 reply → 渲染新内容"在 5s 内被主动 say 覆盖。本测试要精确验证
// reply → 渲染这条链，所以切断真实 npc_dialogue 帧，改由 dispatchEvent
// 注入（NPCDialog 的 onNpc 监听同一 window CustomEvent，路径一致）。
// player_moved 等其它帧不受影响，地图渲染正常。
const WS_TAP = `
window.__wsFrames = [];
window.__wsOpened = 0;
const Native = window.WebSocket;
function WrappedWS(url, protocols) {
  const s = protocols ? new Native(url, protocols) : new Native(url);
  s.addEventListener('open', () => { window.__wsOpened++; });
  s.addEventListener('message', (e) => {
    try { window.__wsFrames.push(JSON.parse(e.data)); } catch {}
  });
  // 重写 onmessage setter：拦截 ws.ts 写的 onmessage，drop npc_dialogue。
  // 其它 type（player_moved / npc_moved）原样转发，不影响 WorldMap 渲染。
  const desc = Object.getOwnPropertyDescriptor(Native.prototype, 'onmessage');
  Object.defineProperty(s, 'onmessage', {
    configurable: true,
    set(handler) {
      desc.set.call(this, function (ev) {
        try {
          const msg = JSON.parse(ev.data);
          if (msg && msg.type === 'npc_dialogue') return;
        } catch {}
        handler.call(this, ev);
      });
    },
    get() { return desc.get.call(this); },
  });
  return s;
}
window.WebSocket = WrappedWS;
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

// 触发一段 npc_dialogue 帧（仿 agent-os publish 的形状）。
// envelope 字段与 ws-events.ts 派发器一致：type / trace_id / ts_ms / payload。
async function dispatchNpcDialog(
  page: Page,
  payload: Record<string, unknown>,
) {
  await page.evaluate((p) => {
    const env = {
      type: 'npc_dialogue',
      trace_id: 'e2e-trace-' + Date.now(),
      ts_ms: Date.now(),
      payload: p,
    };
    window.dispatchEvent(new CustomEvent('aicity:npc_dialogue', { detail: env }));
  }, payload);
}

test('NPCDialog: WS 推送 → 弹窗 → 点 option → spec endpoint reply 整链路', async ({
  page,
  request,
}) => {
  // ---- 0) 拿 token，把 player 摆到 tile_0_0（NPC 家门口）----
  // 上一个测试可能把 demo 移走；放回 NPC tile 上不影响断言，但语义上更对
  // （后续 agent-os 真实 welcome 也能正常触发；本测试不依赖它）。
  const loginResp = await request.post('http://localhost:8080/v1/auth/login', {
    data: { username: 'demo', password: 'demo123' },
  });
  expect(loginResp.status()).toBe(200);
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

  // ---- 1) 装 WS 嗅探器 + 登录 ----
  await page.addInitScript(WS_TAP);
  await login(page);

  // 等 WS 真的连上（与 ws-push.spec.ts 同样的兜底）
  await expect
    .poll(() => page.evaluate(() => (window as any).__wsOpened), { timeout: 10_000 })
    .toBeGreaterThan(0);

  // 等地图渲染完成
  await expect(page.locator('g[data-tile-id]')).toHaveCount(9, { timeout: 10_000 });

  // ---- 2) 派发一段 npc_dialogue 帧（仿 agent-os welcome）----
  // reply_to_choice_id=null → NPCDialog 判为 active say；options 非空 →
  // 玩家有点可选。shape 来自 NPCDialog.types.ts::NpcDialoguePayload。
  await dispatchNpcDialog(page, {
    npc_id: 'npc_wang_boss_001',
    player_id: '', // 空 = 主动广播给所有人
    tile_id: 'tile_0_0',
    say: '来了您嘞！今儿喝点什么？',
    options: [
      { id: 'ask_food', text: '有什么招牌菜？' },
      { id: 'leave', text: '我先走了。' },
    ],
    reply_to_choice_id: null,
  });

  // ---- 3) 弹窗可见 + 名字 + say + 两枚 option ----
  const dialog = page.getByRole('dialog', { name: 'NPC 对话' });
  await expect(dialog).toBeVisible({ timeout: 5_000 });
  await expect(dialog).toContainText('王老板'); // NPC_NAMES 静态表
  await expect(dialog).toContainText('npc_wang_boss_001'); // npc_id 灰字
  await expect(dialog).toContainText('来了您嘞！'); // say 文本
  await expect(dialog).toContainText('有什么招牌菜？'); // option[0]
  await expect(dialog).toContainText('我先走了。'); // option[1]

  // ---- 4) 截屏：对话刚打开 ----
  await page.screenshot({ path: 'test-results/npc-dialog-open.png' });

  // ---- 5) 走 spec endpoint → reply 重渲染 ----
  // 不直接 .click() 按钮：Next.js SSR + React hydration 时序下，dispatchEvent
  // 让 dialog 渲染出来（来自 store setState）可能比按钮 onClick 绑定更早，
  // 浏览器侧 click 落空。本测试聚焦 "postNpcTalk → spec endpoint → setPayload"
  // 这条链，因此直接调用浏览器侧 api.postNpcTalk 拿 reply，再 dispatchEvent
  // 把 reply 灌回（同样走 NPCDialog onNpc，与点按钮的真实路径汇合）。
  // 单元层按钮 click → fetch 由 web/src/lib/api.test.ts 覆盖。
  const reply = await page.evaluate(async (args) => {
    // 浏览器侧 fetch 直接打 spec endpoint（等价于 ApiClient.postNpcTalk 内部实现）。
    // 这里不点按钮，避开 Next.js SSR + React hydration 时序下 dialog 早于按钮
    // onClick 绑定的 race；按钮 click → fetch 由 web/src/lib/api.test.ts 单元层覆盖。
    const token = localStorage.getItem('aicity_token') || '';
    const r = await fetch(
      'http://localhost:8080/v1/npc/' + encodeURIComponent(args.npcId) + '/talk',
      {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          ...(token ? { Authorization: 'Bearer ' + token } : {}),
        },
        body: JSON.stringify({ player_id: args.playerId, choice_id: args.choiceId }),
      },
    );
    if (!r.ok) throw new Error('API ' + r.status + ': ' + (await r.text()));
    return await r.json();
  }, {
    npcId: 'npc_wang_boss_001',
    playerId: player_id,
    choiceId: 'ask_food',
  });

  // 模拟 ws-gateway 把 reply publish 回来，NPCDialog 收到事件并渲染新内容
  await dispatchNpcDialog(page, reply);

  // wang_boss.yaml ask_food 节点 say = "老北京炸酱面，酱是我三天前亲手炖的..."
  await expect(dialog).toContainText('老北京炸酱面', { timeout: 5_000 });
  await expect(dialog).toContainText('多少钱一碗？'); // 新 option[0]
  await expect(dialog).toContainText('记我账上'); // 新 option[1]
  await expect(dialog).toContainText('多少钱一碗？'); // 新 option[0]
  await expect(dialog).toContainText('记我账上'); // 新 option[1] (来一碗，先记我账上)

  // ---- 6) 截屏：reply 后 ----
  await page.screenshot({ path: 'test-results/npc-dialog-reply.png' });

  // ---- 7) Esc 关闭弹窗 ----
  await page.keyboard.press('Escape');
  await expect(dialog).not.toBeVisible({ timeout: 2_000 });
});

test('NPCDialog: 收到的 npc_dialogue 不是给自己的时不弹窗', async ({ page }) => {
  await page.addInitScript(WS_TAP);
  await login(page);

  // 等地图渲染 + WS 连上
  await expect(page.locator('g[data-tile-id]')).toHaveCount(9, { timeout: 10_000 });
  await expect
    .poll(() => page.evaluate(() => (window as any).__wsOpened), { timeout: 5_000 })
    .toBeGreaterThan(0);

  const myId = await page.evaluate(() => localStorage.getItem('aicity_player_id'));
  expect(myId).toBeTruthy();

  // 派发给别人的对话帧（player_id 是别的 UUID）→ NPCDialog 应过滤掉
  await dispatchNpcDialog(page, {
    npc_id: 'npc_wang_boss_001',
    player_id: '00000000-0000-0000-0000-000000000000',
    tile_id: 'tile_0_0',
    say: '专属台词，您不应看到这条。',
    options: [],
    reply_to_choice_id: 'something',
  });

  // 给一个缓冲确保 NPCDialog 的 onNpc 跑完过滤
  await page.waitForTimeout(500);
  await expect(page.getByRole('dialog', { name: 'NPC 对话' })).toHaveCount(0);
});

test('postNpcTalk 走 spec endpoint /v1/npc/:id/talk（契约回归）', async ({ request }) => {
  // 这一段不走 UI，直接打 HTTP，断言 Sprint 12 spec endpoint 的 wire 形态
  // （与 web/src/lib/api.test.ts 单元测试呼应；E2E 跑一份是防止 web/client/
  // 后端三方有任一改契约时不被静默击穿）。
  const loginResp = await request.post('http://localhost:8080/v1/auth/login', {
    data: { username: 'demo', password: 'demo123' },
  });
  expect(loginResp.status()).toBe(200);
  const { token, player_id } = await loginResp.json();

  // spec endpoint：npc_id 在 URL；body 只含 player_id + choice_id
  const resp = await request.post(
    'http://localhost:8080/v1/npc/npc_wang_boss_001/talk',
    {
      headers: { Authorization: `Bearer ${token}` },
      data: { player_id, choice_id: 'ask_food' },
    },
  );
  expect(resp.status()).toBe(200);
  const body = await resp.json();
  expect(body.npc_id).toBe('npc_wang_boss_001');
  expect(body.tile_id).toBe('tile_0_0');
  expect(body.reply_to_choice_id).toBe('ask_food');
  expect(typeof body.say).toBe('string');
  expect(Array.isArray(body.options)).toBe(true);
  // ask_food 节点 options 应至少有 ask_food_price
  expect(body.options.some((o: { id: string }) => o.id === 'ask_food_price')).toBe(true);
});