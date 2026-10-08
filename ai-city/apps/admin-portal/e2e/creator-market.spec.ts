import { createHmac } from 'node:crypto';
import { test, expect, type Cookie, type Page } from '@playwright/test';
import { Client } from 'pg';

const baseURL = process.env.E2E_BASE_URL ?? 'http://localhost:8081';
const dbUrl =
  process.env.DATABASE_URL ??
  'postgresql://aicity:aicity_dev@127.0.0.1:5432/aicity';
const jwtSecret =
  process.env.ADMIN_PORTAL_JWT_SECRET ??
  process.env.JWT_SECRET ??
  'dev-secret-change-me';

test.describe.serial('creator marketplace', () => {
  test.setTimeout(60_000);

  test('creator creates an NPC template and buyer purchase increases revenue', async ({
    browser,
  }) => {
    const { creatorId, buyerId } = await resolveSeedPlayers();
    const creatorToken = signMarketplaceToken({
      sub: creatorId,
      username: 'creator_demo',
      role: 'creator',
    });
    const buyerToken = signMarketplaceToken({
      sub: buyerId,
      username: 'admin',
      role: 'admin',
    });

    const creatorContext = await browser.newContext({ baseURL });
    const buyerContext = await browser.newContext({ baseURL });
    await creatorContext.addCookies([
      sessionCookie(baseURL, creatorToken),
    ]);
    await buyerContext.addCookies([sessionCookie(baseURL, buyerToken)]);

    const creatorPage = await creatorContext.newPage();
    const revenueUrl = `/api/marketplace/revenue/${creatorId}?limit=100&offset=0`;
    const revenueBefore = await readRevenueTotal(creatorPage, revenueUrl);

    const templateName = `E2E NPC ${Date.now()}`;
    await creatorPage.goto('/creator/npc-templates/new');
    await creatorPage.getByLabel('名称').fill(templateName);
    await creatorPage.getByLabel('售价（Gold）').fill('23');

    const createResponse = creatorPage.waitForResponse(
      (response) =>
        response.url().endsWith('/api/marketplace/npc-templates') &&
        response.request().method() === 'POST',
    );
    await creatorPage.getByRole('button', { name: '创建模板' }).click();
    const response = await createResponse;
    expect(response.status()).toBe(201);
    const { id: templateId } = (await response.json()) as { id: number };
    expect(templateId).toBeGreaterThan(0);

    await expect(creatorPage).toHaveURL(/\/creator\/npc-templates$/);
    await expect(creatorPage.getByText(templateName)).toBeVisible();

    const buyerPage = await buyerContext.newPage();
    await buyerPage.goto(`/market/npc-templates/${templateId}`);
    await expect(
      buyerPage.getByRole('heading', { name: templateName }),
    ).toBeVisible();
    await buyerPage.getByRole('button', { name: '购买' }).click();
    await expect(buyerPage.getByText('购买成功')).toBeVisible();

    const revenueAfter = await readRevenueTotal(creatorPage, revenueUrl);
    expect(revenueAfter - revenueBefore).toBe(23);

    await buyerContext.close();
    await creatorContext.close();
  });
});

async function resolveSeedPlayers(): Promise<{
  creatorId: string;
  buyerId: string;
}> {
  const client = new Client({ connectionString: dbUrl });
  await client.connect();
  try {
    const result = await client.query<{
      id: string;
      username: string;
      role: string;
    }>(
      "SELECT id, username, role FROM player WHERE username IN ('creator_demo', 'admin')",
    );
    const creator = result.rows.find((row) => row.username === 'creator_demo');
    const buyer = result.rows.find((row) => row.username === 'admin');
    if (creator?.role !== 'creator' || !buyer) {
      throw new Error(
        'seeded marketplace identities missing; run db/seed/seed-creator-market.sql',
      );
    }
    return { creatorId: creator.id, buyerId: buyer.id };
  } finally {
    await client.end();
  }
}

function signMarketplaceToken(claims: {
  sub: string;
  username: string;
  role: 'creator' | 'admin';
}): string {
  const header = Buffer.from(
    JSON.stringify({ alg: 'HS256', typ: 'JWT' }),
  ).toString('base64url');
  const payload = Buffer.from(
    JSON.stringify({
      ...claims,
      exp: Math.floor(Date.now() / 1000) + 3600,
    }),
  ).toString('base64url');
  const signature = createHmac('sha256', jwtSecret)
    .update(`${header}.${payload}`)
    .digest('base64url');
  return `${header}.${payload}.${signature}`;
}

function sessionCookie(baseURL: string, token: string): Cookie {
  const { hostname, protocol } = new URL(baseURL);
  const expires = Math.floor(Date.now() / 1000) + 3600;
  return {
    name: 'aicity_token',
    value: token,
    domain: hostname,
    path: '/',
    httpOnly: true,
    secure: protocol === 'https:',
    sameSite: 'Lax',
    expires,
  };
}

async function readRevenueTotal(
  page: Page,
  url: string,
): Promise<number> {
  const response = await page.request.get(url);
  expect(response.status()).toBe(200);
  const revenue = (await response.json()) as Array<{
    amount_gold: number | string;
  }>;
  return revenue.reduce(
    (total, item) => total + Number(item.amount_gold),
    0,
  );
}
