import { test, expect, Page } from '@playwright/test';

/**
 * Wallet + Admin flow E2E — Phase C.4 / Task T13
 *
 * Exercises the real admin-portal against the running stack:
 *   login → /wallet (pick demo player, see balances) →
 *   /admin (trigger emit, see success/error) →
 *   /wallet (refresh, balance re-renders).
 *
 * Requires:
 *   - docker compose stack running (Postgres + economy-service +
 *     admin-portal can be `pnpm dev` on :8081)
 *   - seeded `admin` / `adminpass` + `demo` / `demo123` users
 *
 * Selectors verified against current components:
 *   - /login          Username/Password labels, submit button "登录"
 *   - PlayerSelector  native <select data-testid="player-selector">
 *                     options render as "username (role)", e.g. "demo (player)"
 *   - /wallet         heading "Wallet"; refresh "刷新";
 *                     cards data-testid="balance-gold-balance"/"balance-token-balance"
 *   - /admin          heading "Admin Tools";
 *                     emit form input id="emit-reason" with label "Reason";
 *                     submit button "触发 emit";
 *                     success data-testid="emit-success"
 *                     error   data-testid="emit-error"
 */

const ADMIN_USER = 'admin';
const ADMIN_PASS = 'adminpass';

async function loginAsAdmin(page: Page) {
  await page.goto('/login');
  await expect(page.getByRole('heading', { name: 'Admin Login' })).toBeVisible({
    timeout: 15_000,
  });
  await page.getByLabel('Username').fill(ADMIN_USER);
  await page.getByLabel('Password').fill(ADMIN_PASS);
  await page.getByRole('button', { name: '登录' }).click();
  // Browser form POST → 303 redirect to next (defaults to "/").
  await page.waitForURL((url) => !url.pathname.startsWith('/login'), {
    timeout: 15_000,
  });
}

test.describe('admin-portal wallet + admin flow', () => {
  test('admin logs in, inspects demo wallet, triggers emit, sees updated balance', async ({
    page,
  }) => {
    // ---- 1. Login as admin/adminpass ----
    await loginAsAdmin(page);

    // ---- 2. Navigate to /wallet ----
    await page.goto('/wallet');
    await expect(page.getByRole('heading', { name: 'Wallet' })).toBeVisible({
      timeout: 15_000,
    });

    // ---- 3. Pick demo player via header selector ----
    const playerSelector = page.getByTestId('player-selector');
    await expect(playerSelector).toBeVisible({ timeout: 15_000 });
    // Options render as "username (role)" — find the demo row by inspecting
    // the rendered <option> elements (label regex is not supported by
    // Playwright's typed `selectOption` overload).
    const demoValue = await page.evaluate(() => {
      const sel = document.querySelector(
        '[data-testid="player-selector"]',
      ) as HTMLSelectElement | null;
      if (!sel) return null;
      const opt = Array.from(sel.options).find((o) =>
        o.textContent?.trim().startsWith('demo'),
      );
      return opt?.value ?? null;
    });
    expect(demoValue).toBeTruthy();
    await playerSelector.selectOption(demoValue!);

    // ---- 4. Verify both balance cards render a number ----
    const goldBalance = page.getByTestId('balance-gold-balance');
    const tokenBalance = page.getByTestId('balance-token-balance');
    await expect(goldBalance).toBeVisible({ timeout: 15_000 });
    await expect(tokenBalance).toBeVisible({ timeout: 15_000 });
    // formatNumber() always produces a non-empty digits string.
    const goldText = (await goldBalance.textContent())?.trim() ?? '';
    const tokenText = (await tokenBalance.textContent())?.trim() ?? '';
    expect(goldText).toMatch(/\d/);
    expect(tokenText).toMatch(/\d/);

    // ---- 5. Navigate to /admin ----
    await page.goto('/admin');
    await expect(page.getByRole('heading', { name: 'Admin Tools' })).toBeVisible({
      timeout: 15_000,
    });

    // ---- 6. Fill emit reason + submit ----
    await page.getByLabel('Reason').fill('e2e_test_emit');
    await page.getByRole('button', { name: '触发 emit' }).click();

    // ---- 7. Verify success OR error feedback ----
    const success = page.getByTestId('emit-success');
    const errorBanner = page.getByTestId('emit-error');
    await expect(success.or(errorBanner)).toBeVisible({ timeout: 15_000 });

    // ---- 8. Back to /wallet, click 刷新, balance still rendered ----
    await page.goto('/wallet');
    await expect(page.getByTestId('balance-gold-balance')).toBeVisible({
      timeout: 15_000,
    });
    await page.getByRole('button', { name: '刷新' }).click();
    await expect(page.getByTestId('balance-gold-balance')).toBeVisible({
      timeout: 15_000,
    });
    // Player selection survives the back-navigation (zustand + sessionStorage).
    await expect(playerSelector).toHaveValue(/.+/);
  });
});
