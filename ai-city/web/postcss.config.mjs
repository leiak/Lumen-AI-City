// Next.js 15 + Tailwind v3 必备 —— 没有这个文件，@tailwind utilities 不会展开，
// 所有 utility class（h-screen / relative / w-full 等）实际不生效。
// 表现：外层 div 用 h-screen 期望占满 100vh，实际只有内容自然高度。
// 副作用：WorldMap SVG 内的 click bbox 漂移，e2e 点击命中点和 viewBox 中心对不上。
const config = {
  plugins: {
    tailwindcss: {},
    autoprefixer: {},
  },
};

export default config;