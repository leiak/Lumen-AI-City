# ============================================
# web (Next.js 15 + pnpm workspace) —— 构建上下文 = ai-city/
# ============================================
# 刻意用单 stage，不做 deps/builder/runner 三段：pnpm 的 symlink farm 在 stage
# 之间搬 node_modules 极脆。单 stage 里 install 与 build 共享同一份布局，风险归零。
# 代价是镜像含 dev 依赖偏大（dev 环境可接受；pnpm prune --prod 留作后续优化）。

FROM node:22-alpine
WORKDIR /app

RUN corepack enable

# --- 依赖层：先只拷 manifest + lockfile，源码改动不失效这一层 ---
# pnpm-lock.yaml 有 importer（. / apps/admin-portal / packages/offline-sync /
# packages/sdk-ts / web），--frozen-lockfile 会核对 importer 与磁盘 package.json
# 集合 → 这些（外加 packages/bypass-filter）的 manifest 都要拷进来。
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY apps/admin-portal/package.json apps/admin-portal/package.json
COPY packages/offline-sync/package.json packages/offline-sync/package.json
COPY packages/bypass-filter/package.json packages/bypass-filter/package.json
COPY packages/sdk-ts/package.json packages/sdk-ts/package.json
COPY web/package.json web/package.json

RUN pnpm install --frozen-lockfile

# --- 源码层 ---
# workspace 包的 package.json 已改成 main/types 指向 src/index.ts，
# 且 web/next.config.js 把它们全列进 transpilePackages → Next 直接编译 TS 源码。
COPY packages/offline-sync packages/offline-sync
COPY packages/bypass-filter packages/bypass-filter
COPY packages/sdk-ts packages/sdk-ts
COPY web web
# 不写 COPY web/public：该目录不存在，COPY 缺失路径是硬失败

# --- 构建 ---
# NEXT_PUBLIC_* 是 build-time inline（web/next.config.js 的 env: 块），
# 必须走 build args 且必须是 host 可达 URL（浏览器端消费）。
ARG NEXT_PUBLIC_API_GATEWAY=http://localhost:8080
ARG NEXT_PUBLIC_WS_GATEWAY=ws://localhost:8082/ws
ENV NEXT_PUBLIC_API_GATEWAY=$NEXT_PUBLIC_API_GATEWAY
ENV NEXT_PUBLIC_WS_GATEWAY=$NEXT_PUBLIC_WS_GATEWAY
ENV NEXT_TELEMETRY_DISABLED=1

RUN pnpm --filter web build

WORKDIR /app/web
ENV NODE_ENV=production
EXPOSE 3000
# next start -p 3000 绑 0.0.0.0
CMD ["pnpm", "start"]
