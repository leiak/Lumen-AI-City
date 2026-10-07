import { defineConfig } from 'vitest/config';
import { resolve } from 'node:path';

export default defineConfig({
  resolve: {
    alias: {
      '@': resolve(__dirname, 'src'),
    },
  },
  esbuild: {
    // React 17+ automatic JSX runtime — no need to `import React` in every .tsx
    jsx: 'automatic',
  },
  test: {
    // happy-dom provides sessionStorage / window / document for tests that
    // touch browser APIs (zustand persist → sessionStorage).
    environment: 'happy-dom',
    // Extend expect with @testing-library/jest-dom matchers (toBeInTheDocument, etc.)
    setupFiles: ['./tests/setup.ts'],
    // Exclude Playwright E2E tests — they run via `pnpm playwright test`
    exclude: ['**/node_modules/**', '**/dist/**', 'e2e/**'],
  },
});
