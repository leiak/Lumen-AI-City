/**
 * 地图端到端：login → /city → 看见 9 个 tile → 点击 move → 看到自己移动
 *
 * 前提：6 容器（postgres / redis / world-engine / api-gateway / a2a-gateway / web）
 * 都在跑，web 暴露在 127.0.0.1:3000。
 */
import { expect, test } from '@playwright/test';

// API base — docker compose 把 api-gateway 暴露在 8080。
// 直接用 request fixture 跑 setup（不走浏览器）。
test('login → /city → 9 tiles visible → click move → position changed', async ({ page, request }) => {
  // ---- 0) 前置：把 demo 玩家重置到 tile_0_0 (50,50) ----
  // 上一次测试可能把玩家移到任意 tile，且 tile_1_0 上常有 grpc_smoke_player_001
  // 干扰；先把它踢回已知位置再断言。
  const login = await request.post('http://localhost:8080/v1/auth/login', {
    data: { username: 'demo', password: 'demo123' },
  });
  expect(login.status()).toBe(200);
  const { token, player_id } = await login.json();
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

  // ---- 1) 登录 ----
  await page.goto('/login');
  await expect(page.getByRole('heading', { name: 'AI 城邦' })).toBeVisible();

  // label 没绑 htmlFor，用 type 选择器；表单已经预填 demo/demo123，覆盖之
  await page.locator('input[type="text"]').first().fill('demo');
  await page.locator('input[type="password"]').first().fill('demo123');
  await page.locator('button:has-text("登录")').click();

  // login 成功后 router.push('/city')
  await page.waitForURL('**/city', { timeout: 10_000 });

  // ---- 2) /city 渲染完成：SVG 出现 + 9 个 tile ----
  const svg = page.locator('svg').first();
  await expect(svg).toBeVisible({ timeout: 10_000 });

  // 等首次 fetchTiles 完成（出现 9 个 rect 算完成）
  const tiles = page.locator('g[data-tile-id]');
  await expect(tiles).toHaveCount(9, { timeout: 10_000 });

  // LOD 颜色 spot-check
  await expect(page.locator('g[data-tile-id="tile_0_0"] rect')).toBeVisible();
  await expect(page.locator('g[data-tile-id="tile_-1_1"] rect')).toBeVisible();
  await expect(page.locator('g[data-tile-id="tile_1_0"] rect')).toBeVisible();

  // ---- 3) 静态建筑 spot-check（pg-schema.sql 种子数据）----
  // tile_0_0 有 2 座（Tavern + Plaza）
  await expect(
    page.locator('polygon[data-building-id="bldg_tavern_0_0"]'),
  ).toBeVisible();
  await expect(
    page.locator('polygon[data-building-id="bldg_plaza_0_0"]'),
  ).toBeVisible();
  // tile_-1_1 有 1 座（Park）
  await expect(
    page.locator('polygon[data-building-id="bldg_park_-1_1"]'),
  ).toBeVisible();

  // ---- 4) NPC 圆点 ----
  // tile_0_0 有 npc_wang_boss_001
  await expect(
    page.locator('circle[data-npc-id="npc_wang_boss_001"]'),
  ).toBeVisible();

  // ---- 5) 自己的圆点（蓝色 r=5）----
  const meMarker = page.locator('circle[data-player-id]').first();
  await expect(meMarker).toBeVisible();

  // ---- 6) 读 HUD 拿到初始坐标 ----
  const hud = page.locator('text=/坐标:/').first();
  await expect(hud).toBeVisible();
  const initialHud = (await hud.textContent()) ?? '';
  const initialMatch = initialHud.match(/坐标:\s*(-?\d+),\s*(-?\d+)/);
  expect(initialMatch, `HUD 格式不对: "${initialHud}"`).not.toBeNull();
  const initialX = Number(initialMatch![1]);
  const initialY = Number(initialMatch![2]);
  console.log(`initial HUD: x=${initialX}, y=${initialY}`);

  // ---- 7) 截屏：移动前 ----
  await page.screenshot({ path: 'test-results/map-before.png' });

  // ---- 8) 选一个"永远空"的目标 tile ----
  // tile_1_0 常驻 grpc_smoke_player_001，tile_0_0 自己可能在那 —— 用 tile_-1_0
  // （始终无 NPC / player / building）
  const moveToTile = 'tile_-1_0';
  const expectedPos = { x: -50, y: 50 };
  console.log(`will click ${moveToTile} center (${expectedPos.x}, ${expectedPos.y})`);

  // 用 dispatchEvent + MouseEvent 直接打 click —— 跳过 Playwright 的 hit-test 拦截
  // （tile_0_0 中心也有 npc 圆点，统一绕开）。bubbles 让 svg 上的 onClick 收到事件，
  // e.target = tileG（tagName='g'，不进 polygon/circle 早返回分支）
  await page.evaluate((tileId) => {
    const svg = document.querySelector('svg')!;
    const tileG = svg.querySelector(`g[data-tile-id="${tileId}"]`) as SVGGElement;
    const r = tileG.getBoundingClientRect();
    const ev = new MouseEvent('click', {
      bubbles: true,
      cancelable: true,
      view: window,
      clientX: r.x + r.width / 2,
      clientY: r.y + r.height / 2,
    });
    tileG.dispatchEvent(ev);
  }, moveToTile);

  // ---- 9) 等 HUD 变化到目标 tile 中心附近 ----
  await expect(async () => {
    const txt = (await page.locator('text=/坐标:/').first().textContent()) ?? '';
    const m = txt.match(/坐标:\s*(-?\d+(?:\.\d+)?),\s*(-?\d+(?:\.\d+)?)/);
    expect(m, `HUD 解析失败: "${txt}"`).not.toBeNull();
    const x = Number(m![1]);
    const y = Number(m![2]);
    // 服务端会精确回写目标坐标（world-engine grpc.rs:114-117 直接返 target.x/y，
    // 不做碰撞调整）→ 容差 1.0 足够
    expect(
      Math.abs(x - expectedPos.x) < 1 && Math.abs(y - expectedPos.y) < 1,
      `HUD 没到目标 (${expectedPos.x}, ${expectedPos.y}): actual (${x}, ${y})`,
    ).toBe(true);
  }).toPass({ timeout: 5_000, intervals: [200, 500, 1000] });

  // ---- 10) 截屏：移动后 ----
  await page.screenshot({ path: 'test-results/map-after.png' });

  // ---- 11) 等 3s 轮询：tile.player_ids 应该反映新位置 ----
  // 这里没法直接断言 player_ids（不在 DOM 里），但 /v1/tiles 至少不会报错
  await page.waitForTimeout(3_500);
  await expect(page.locator('g[data-tile-id]')).toHaveCount(9);
});

test('login failure shows error message', async ({ page }) => {
  await page.goto('/login');
  await page.locator('input[type="text"]').first().fill('demo');
  await page.locator('input[type="password"]').first().fill('wrongpassword');
  await page.locator('button:has-text("登录")').click();
  // 错误条：bg-red-900/30 容器，text-sm 文本；ApiClient 抛 "API 401: ..."
  await expect(page.locator('.bg-red-900\\/30')).toBeVisible({ timeout: 5_000 });
  await expect(page.locator('.bg-red-900\\/30')).toContainText(/API|Failed/i);
});
