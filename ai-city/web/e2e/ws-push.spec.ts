import { test, expect, type Page } from '@playwright/test';

/**
 * Sprint 9：验证 WorldMap 靠 WS 推送而非轮询更新。
 *
 * 断言两件事：
 *  1. 进 /city 后 WS 真的连上（startWsBridge → ws.connect）
 *  2. 移动后收到 player_moved 信封，且 payload.player_id 是自己
 *
 * 用 page.evaluate 挂一个 WebSocket 帧嗅探器（在 addInitScript 里包装原生
 * WebSocket），比断言 DOM 更直接 —— DOM 更新还要过 debounce + fetch。
 */

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

// label 没绑 htmlFor，getByLabel 解析不到 → 用 type 选择器（与 map-flow.spec.ts 一致）
async function login(page: Page) {
  await page.goto('/login');
  await page.locator('input[type="text"]').first().fill('demo');
  await page.locator('input[type="password"]').first().fill('demo123');
  await page.locator('button:has-text("登录")').click();
  await page.waitForURL('**/city');
}

test('进 /city 后 WS 连上，移动触发 player_moved 推送', async ({ page, request }) => {
  // 前置：把 demo 踢回 tile_0_0 (50,50)。下面要断言"移动到 tile_-1_0"，
  // 若上一次测试已经把它留在 tile_-1_0，move 仍会成功但语义就不成立了。
  const loginResp = await request.post('http://localhost:8080/v1/auth/login', {
    data: { username: 'demo', password: 'demo123' },
  });
  expect(loginResp.status()).toBe(200);
  const { token, player_id } = await loginResp.json();
  await request.post('http://localhost:8080/v1/world/move', {
    headers: { Authorization: `Bearer ${token}` },
    data: { player_id, from_tile_id: 'tile_0_0', to_tile_id: 'tile_0_0', x: 50, y: 50 },
  });

  await page.addInitScript(WS_TAP);
  await login(page);

  // 1) WS 连上
  await expect
    .poll(() => page.evaluate(() => (window as any).__wsOpened), { timeout: 10_000 })
    .toBeGreaterThan(0);

  const playerId = await page.evaluate(() => localStorage.getItem('aicity_player_id'));
  expect(playerId).toBeTruthy();

  // 等地图渲染出 9 个 tile
  await expect(page.locator('[data-tile-id]')).toHaveCount(9, { timeout: 10_000 });

  // 2) 在一个 tile 中心 dispatch click 发 move。
  //    不用 .click()：tile 中心常驻 NPC / 玩家圆点会拦截 hit-test（Sprint 8 已知坑）。
  await page.evaluate(() => {
    const g = document.querySelector('[data-tile-id="tile_-1_0"]') as SVGGElement;
    const r = g.getBoundingClientRect();
    g.dispatchEvent(
      new MouseEvent('click', {
        bubbles: true,
        clientX: r.x + r.width / 2,
        clientY: r.y + r.height / 2,
      }),
    );
  });

  // 3) 收到自己的 player_moved 信封（走 world-engine → Redis → ws-gateway）
  await expect
    .poll(
      () =>
        page.evaluate((pid) => {
          const frames = (window as any).__wsFrames as any[];
          return frames.some(
            (f) => f?.type === 'player_moved' && f?.payload?.player_id === pid,
          );
        }, playerId),
      { timeout: 10_000 },
    )
    .toBe(true);

  // 信封字段齐全（trace_id / ts_ms / payload.tile_id）
  const env = await page.evaluate((pid) => {
    const frames = (window as any).__wsFrames as any[];
    return frames.find((f) => f?.type === 'player_moved' && f?.payload?.player_id === pid);
  }, playerId);

  expect(env.trace_id).toBeTruthy();
  expect(env.ts_ms).toBeGreaterThan(0);
  expect(env.payload.tile_id).toBe('tile_-1_0');
});
