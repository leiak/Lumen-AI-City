import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    // Exclude Playwright E2E tests — they run via `pnpm playwright test`
    exclude: ['**/node_modules/**', '**/dist/**', 'e2e/**'],
  },
});