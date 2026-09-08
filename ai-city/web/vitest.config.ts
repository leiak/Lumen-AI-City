import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';
import { resolve } from 'path';

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': resolve(__dirname, './src'),
    },
  },
  test: {
    environment: 'node',
    include: ['src/**/*.test.{ts,tsx}'],
    globals: false,
    // jsdom 已被 @testing-library/react 的 peer 解析进来；组件测试通过文件顶
    // `// @vitest-environment jsdom` 切到 DOM env，lib 单测仍走 node 保持快速。
    // setupFiles 提供 jest-dom matchers（toBeInTheDocument 等）。
    setupFiles: ['./vitest.setup.ts'],
  },
});
