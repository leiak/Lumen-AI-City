import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  testDir: './e2e',
  fullyParallel: false, // 共享同一份 PG / Redis，串行更稳
  workers: 1,
  reporter: 'list',
  timeout: 30_000,
  expect: { timeout: 8_000 },
  use: {
    // 用 localhost 而非 127.0.0.1：api-gateway 的 CORS allowlist 是
    // http://localhost:3000（见 apps/api-gateway/internal/config/config.go）
    baseURL: 'http://localhost:3000',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    headless: true,
    viewport: { width: 1280, height: 800 },
  },
  projects: [
    { name: 'chromium', use: { ...devices['Desktop Chrome'] } },
  ],
});
