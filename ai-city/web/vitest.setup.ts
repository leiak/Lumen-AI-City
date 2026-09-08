/**
 * Vitest 全局 setup（仅 DOM 测试用，lib 单测在 node env 不需要）
 *
 * 加载 @testing-library/jest-dom 让 toBeInTheDocument / toHaveTextContent 等
 * 断言在 Vitest 的 expect 上可用（Vitest 不像 Jest 默认集成 chai-dom）。
 */
// 必须在 import matchers 之后再 import expect-then 扩展
import '@testing-library/jest-dom/vitest';
