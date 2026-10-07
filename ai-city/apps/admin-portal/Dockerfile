FROM node:22-alpine AS deps
WORKDIR /app
# Build admin-portal as a standalone package — copy its package.json + lockfile only
COPY apps/admin-portal/package.json apps/admin-portal/pnpm-lock.yaml* ./
# Pin pnpm to match workspace root packageManager (pnpm@9.12.0).
# Without pinning, corepack falls back to the latest pnpm (v11+) whose
# default minimumReleaseAge=1440min blocks installs of recently published
# packages (e.g. resolve@1.22.13 published 2026-10-06T22Z → cutoff ~16h).
RUN corepack enable && corepack prepare pnpm@9.12.0 --activate && \
    pnpm install --frozen-lockfile --ignore-workspace

FROM node:22-alpine AS builder
WORKDIR /app
RUN corepack enable && corepack prepare pnpm@9.12.0 --activate
COPY --from=deps /app/node_modules ./node_modules
# Copy the entire admin-portal source
COPY apps/admin-portal ./
RUN pnpm build

FROM node:22-alpine AS runner
WORKDIR /app
ENV NODE_ENV=production
COPY --from=builder /app/.next ./.next
COPY --from=builder /app/node_modules ./node_modules
COPY --from=builder /app/package.json ./package.json
EXPOSE 8081
# Invoke next CLI directly — pnpm/corepack are not in the runner image
# (avoids "Cannot find module '/app/pnpm'" at container start).
CMD ["node", "./node_modules/next/dist/bin/next", "start", "-p", "8081"]
