import { defineConfig } from 'vitest/config';
import { resolve } from 'node:path';

export default defineConfig({
  resolve: {
    alias: {
      '@': resolve(__dirname, 'src'),
    },
  },
  test: {
    // happy-dom provides sessionStorage / window / document for tests that
    // touch browser APIs (zustand persist → sessionStorage).
    environment: 'happy-dom',
    // Exclude Playwright E2E tests — they run via `pnpm playwright test`
    exclude: ['**/node_modules/**', '**/dist/**', 'e2e/**'],
  },
});
