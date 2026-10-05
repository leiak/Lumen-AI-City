import { test, expect } from '@playwright/test';

test('LLM-NPC 上下文记忆', async ({ page }) => {
  await page.goto('http://localhost:3000/city');
  await page.click('[data-npc-id="npc_a_wang_boss"]');
  await page.fill('[data-testid="chat-input"]', '今天有什么好吃的？');
  await page.click('[data-testid="chat-send"]');
  await page.waitForSelector('[data-testid="npc-response"]', { timeout: 10000 });
  await page.fill('[data-testid="chat-input"]', '那个红烧肉多少钱？');
  await page.click('[data-testid="chat-send"]');
  const response = await page.textContent('[data-testid="npc-response"]');
  expect(response).toMatch(/红烧肉|价格|多少/);
});

test('跨城 NPC 路由', async ({ page }) => {
  // city_a 玩家点 city_b NPC
  await page.goto('http://localhost:3000/city');
  await page.click('[data-npc-id="npc_b_grace_healer"]');
  await page.fill('[data-testid="chat-input"]', '医生，我失眠');
  await page.click('[data-testid="chat-send"]');
  await page.waitForSelector('[data-testid="npc-response"]', { timeout: 15000 });
  const response = await page.textContent('[data-testid="npc-response"]');
  expect(response).toBeTruthy();
  expect(response!.length).toBeGreaterThan(0);
});
