# ADR-0009: 3.0 v2 — admin-portal 钱包 UI（proxy + zustand + react-query）

**Status:** Accepted (2026-10-07)
**Date:** 2026-10-07
**Deciders:** fang, AI city core team
**Supersedes:** 无（3.0 v2 子轨道 #5 admin-portal 落地 ADR；不修改 ADR-0008 v1 范围）
**前置依赖:**
- [ADR-0008](0008-3.0-economy-scope.md) **Accepted**（3.0 v1 economy-service GA：钱包 + 转账 + 商品 + 中央银行 + Kafka/Redis）
- [ADR-0007](0007-2.0-scope-llm-federation-memory-saga.md) **Accepted**（2.0 阶段 3 Phase C admin-portal + admin auth 已 GA：C.4 admin login + JWT cookie）
- [ADR-0006](0006-1.0-scope-mvp-demo-slice.md) **Accepted**（1.0 MVP Demo 闭环）
**影响范围:**
- `apps/admin-portal/` — 新增 3 routes (/wallet /transactions /admin) + 5 backend API proxies + Layout 重构 + PlayerSelector
- 新增依赖：zustand 5 + @tanstack/react-query 5 + lucide-react + date-fns（仅 devDep）
- `docker-compose.yml` — admin-portal env 接线（ECONOMY_URL / ADMIN_TOKEN / JWT_SECRET / DATABASE_URL）
- `apps/admin-portal/Dockerfile` — builder stage corepack enable + pnpm-lock.yaml 提交（commit `125dc20`）
- `db/seed/` — 沿用 v1 seed-economy-demo.sql（无新迁移）

---

## Context

3.0 v1 economy-service 已 GA（ADR-0008）—— 钱包 / 转账 / 商品购买 / 中央银行 / Kafka / Redis 全部到位，10/10 acceptance 通过，14 容器 healthy。

但 **stakeholder 演示卡壳**：

- 玩家只能通过 `curl` 直接调 economy-service REST（无 UI）
- 运营人员想看 demo 玩家余额 → 必须 psql 查表
- 想触发中央银行 emit/sink → 必须拼 POST + 找 ADMIN_TOKEN
- 交易历史只能看 JSON dump（无表格 / 分页 / 类型色标）

**3.0 v2 子轨道 #5 admin-portal**（[3.0-ROADMAP §后续路线](../3.0-ROADMAP.md#后续路线v2-候选)）= `/wallet` + `/transactions` + `/admin` 三个路由 + 中央银行 dashboard 雏形。本 ADR 记录 v2 GA 决策与实施。

**v2 拒绝**（YAGNI）：
- 玩家间转账 UI（v1 已有 REST；UI 推迟到 v3 / 创作者市场）
- NPC 商品目录浏览（运营仅需 emit/sink，玩家购买走 NPC BT 触发）
- WebSocket 实时事件（react-query manual refresh + invalidation 足够 v2；v3 再升级）
- 移动端响应式（仅 web admin 端）
- 多语言 / 货币切换（v1 仅中文 + 双货币）

---

## Decision

3.0 v2 admin-portal wallet UI = **3 routes + 5 backend proxies + 1 Layout 重构 + 2 状态库**：

### 1. 3 个客户端路由（App Router）

| Route | 功能 | 数据来源 |
|---|---|---|
| `/wallet` | 余额卡片（gold + token）+ 手动刷新 + 错误/loading/empty states | `GET /api/economy/wallet/{id}` |
| `/transactions` | 表格（time/type/currency/amount/balance_after/memo）+ 分页（50/page）+ 类型色标 | `GET /api/economy/transactions/{id}?limit=50&offset=N` |
| `/admin` | emit + sink forms，server 错误码展示，success 后 invalidate wallet+transactions | `POST /api/economy/admin/emit` + `/admin/sink` |

所有路由都是 server component 做 cookie 鉴权 → 包裹 client component（react-query + zustand）。

### 2. 5 个 backend API proxies（隐藏 ADMIN_TOKEN）

| Route | 后端 | Proxy 职责 |
|---|---|---|
| `GET /api/players` | PG（player 表 role='player'）| 列表过滤掉 admin 账号；DB 错误脱敏 |
| `GET /api/economy/wallet/[user_id]` | economy-service `/api/v1/wallet/{id}` | 透传，错误映射 `{error:{code,msg}}` |
| `GET /api/economy/transactions/[user_id]` | economy-service `/api/v1/transactions/{id}?limit=&offset=` | URLSearchParams 重新编码（防 nested encoding bug）|
| `POST /api/economy/admin/emit` | economy-service `/api/v1/admin/central-bank/emit` | 服务端注入 `Authorization: Bearer ${ADMIN_TOKEN}` |
| `POST /api/economy/admin/sink` | economy-service `/api/v1/admin/central-bank/sink` | 同上 + pydantic body 校验 |

**核心安全原则**：浏览器只看到 admin-portal `/api/*` 路径（不直接访问 economy-service），所有 `ADMIN_TOKEN` / `JWT_SECRET` 仅在 server runtime 存在（`process.env.ADMIN_TOKEN`）。

### 3. Layout 重构（sidebar + 顶部 player_id selector）

- `Layout.tsx` — client wrapper（Provider + Sidebar + Header + `{children}`）
- `Sidebar.tsx` — nav links（/ /wallet /transactions /saga-viz /bt-editor /admin）
- `Header.tsx` — top bar（PlayerSelector + 当前 admin user info + 退出）
- `PlayerSelector.tsx` — `<select>` of seeded players，zustand 写 sessionStorage，react-query 加载列表

### 4. 状态管理（zustand + react-query）

**zustand `usePlayerStore`**（`src/lib/usePlayerStore.ts`）：
- `selectedPlayerId: string | null`
- `setSelected(id)` / `clear()`
- sessionStorage persist（key: `aicity.admin.selectedPlayer`）

**react-query**：
- `useQuery(['players'])` — 列表缓存
- `useQuery(['wallet', id])` — 余额，手动 refetch（refresh 按钮）
- `useQuery(['transactions', id, page])` — 交易历史分页
- `useMutation` (emit/sink) → `invalidateQueries(['wallet'])` + `['transactions']`（**不是** `removeQueries`，按 spec 留缓存结构）

### 5. 错误处理 envelope

```ts
// src/lib/economy.ts
type ErrorEnvelope = { error: { code: string; msg: string } };

export function describeError(status: number, code?: string): string {
  // R_018 BAD_REQUEST / R_022 INSUFFICIENT_BALANCE / R_026 ADMIN_REQUIRED ...
  // 503 ECON_UPSTREAM_DOWN (custom for proxy unreachable)
}
```

### 6. 依赖新增（5 个）

| Package | 用途 |
|---|---|
| `zustand@^5` | selectedPlayerId 状态 + persist |
| `@tanstack/react-query@^5` | 数据获取 + 缓存 + invalidation |
| `lucide-react` | icons（RefreshCw / ChevronLeft / ChevronRight）|
| `date-fns` | 时间格式化（zh-CN locale）|
| `@playwright/test` (devDep) | E2E wallet-admin 流程 |

无破坏性新增（admin-portal 已有 next 15 + react 19 + tailwind）。

### 7. Dockerfile 修复（commit `125dc20`）

- builder stage 增加 `corepack enable`（pnpm 不可用 → build fail）
- `pnpm-lock.yaml` 提交（防止 lockfile drift 触发 lockfile-only 依赖未安装）

---

## Components

| 组件 | 路径 | 职责 |
|---|---|---|
| `usePlayerStore` | `src/lib/usePlayerStore.ts` | zustand + sessionStorage persist |
| `economy.ts` | `src/lib/economy.ts` | typed fetch + error mapping (R_018/R_022/R_026/R_503) |
| `Layout` | `src/components/Layout.tsx` | client wrapper（QueryClientProvider + Sidebar + Header）|
| `Sidebar` | `src/components/Sidebar.tsx` | nav links |
| `Header` | `src/components/Header.tsx` | PlayerSelector + admin user + logout |
| `PlayerSelector` | `src/components/PlayerSelector.tsx` | dropdown + zustand 写入 |
| `WalletClient` | `src/app/wallet/WalletClient.tsx` | balance cards + refresh + states |
| `TransactionsClient` | `src/app/transactions/TransactionsClient.tsx` | table + pagination + type labels |
| `AdminClient` | `src/app/admin/AdminClient.tsx` | emit/sink forms + error display + invalidation |
| 5 proxies | `src/app/api/{players,economy/{wallet,transactions,admin/{emit,sink}}}/route.ts` | server-side auth + ECON forwarding |

---

## 关键设计决策

1. **Proxy pattern（server-side Bearer token）**：浏览器永远不接触 economy-service 直连地址 / `ADMIN_TOKEN`；admin-portal `/api/*` 是唯一入口；Open API 边界清晰（cross-origin 不暴露 secret）
2. **zustand + sessionStorage（不是 localStorage / 不是 URL params）**：跨 tab 隔离（避免 admin 在多 tab 选不同玩家）；刷新页面保留；无 auth'd URL params（防 referer leak）
3. **react-query manual refresh（不是 WebSocket）**：v2 YAGNI；refetch button + post-mutation invalidation 覆盖 95% 用例；v3 接 ws-gateway 事件流
4. **invalidateQueries（不是 removeQueries）**：保留 query key 结构（pagination / filter）；下次进入页面可复用 cache shape
5. **error envelope `{error:{code,msg}}`**：与 economy-service R_018~R_026 对齐；proxy 层加 `R_503 ECON_UPSTREAM_DOWN` 处理 ECONNREFUSED
6. **server component 做 cookie 鉴权**：与 Phase C.4 BT editor 一致（`decodeToken + isAdmin` 在 server，避免 edge runtime crypto 限制）
7. **Layout 用 client wrapper 而非 server component**：react-query 需要 `QueryClientProvider`（必须是 client），用 client component 包 `{children}` 不影响 server page 的 RSC 能力

---

## 风险

| 风险 | 缓解 | 状态 |
|---|---|---|
| `ADMIN_TOKEN` 泄漏到浏览器 bundle | proxy pattern（server runtime only）+ Next.js 默认 server runtime；admin-portal 不暴露 token env | ✅ verified via bundle inspection |
| sessionStorage 多 tab 状态不一致 | 接受（运营主动选玩家；多 tab 是反模式）；如需同步 v3 接 broadcast channel | ✅ YAGNI v2 |
| react-query 缓存 stale | `staleTime: 0` + manual refetch；v3 接 WebSocket | ✅ 测试覆盖 |
| economy-service ECONNREFUSED | proxy 层捕获 → 503 R_503 ECON_UPSTREAM_DOWN + describeError | ✅ 测试覆盖 |
| pnpm 在 Docker builder 不可用 | commit `125dc20`：`corepack enable` + 提交 pnpm-lock.yaml | ✅ build pass |
| URLSearchParams 二次编码（nested params） | proxy 层 `new URLSearchParams()` 重新构造（commit `44063a5`）| ✅ tests cover |
| 后端错误码未映射到前端 | describeError helper + R_018/R_022/R_026/R_503 表 | ✅ tests cover |

---

## Consequences

### Positive

- **stakeholder 5min demo 闭环**：admin login → 选 demo → 看到余额 → /admin emit → 返回 /wallet 刷新 → 余额变化；不需要 curl / psql
- **运营自助**：中央银行 emit/sink 触发不再依赖 dev 找 ADMIN_TOKEN
- **错误可观测**：前端 describeError 统一展示 R_018/R_022/R_026/R_503，server 错误码透传
- **可扩展性**：3 routes + 5 proxies + Layout 结构是 admin-portal 未来扩展（玩家间转账 UI / 商品目录 / NPC dashboard）的模板
- **架构清晰**：proxy pattern 把 secrets 隔离在 server；client 只跟 `/api/*` 打交道

### Negative / 风险

- **5 个新依赖**（zustand / react-query / lucide / date-fns / @playwright/test）→ bundle 增 ~50KB（gzip）；可接受
- **zustand 跨 tab 不同步**：多 tab 同时选不同玩家时不会自动同步（YAGNI v2）
- **react-query 无 WebSocket**：余额变更需要手动 refresh（运营点按钮 OK，监控场景不够）
- **proxy 层错误脱敏可能丢失调试信息**：DB 错误在 server log 保留完整 stack，但前端只显示 R_xxx
- **E2E 测试需要完整 stack**（admin-portal + economy-service + postgres）：CI 跑需 docker compose up；smoke mock 模式（`--mock`）做开发态 fallback

### Trade-offs

- **Proxy vs direct fetch**：选 proxy（secret 隔离 + 中央化错误映射）vs direct（少 1 hop latency +5ms）；运营场景接受
- **sessionStorage vs localStorage**：选 sessionStorage（跨 tab 隔离 + 不持久化避免泄漏 demo player UUID）vs localStorage（持久化免去每次登录重选）
- **zustand vs Context**：选 zustand（细粒度订阅 + persist middleware）vs Context（更 React 原生但 re-render 粒度粗）
- **manual refresh vs WebSocket**：选 manual（v2 简单 + 92 tests 通过）vs WebSocket（实时但需要 ws-gateway 集成 + 心跳 + 重连）

---

## 实施计划（已 GA 2026-10-07）

14 个 task（T1-T14）按依赖顺序落地：

| Task | 内容 | Commit |
|---|---|---|
| T1 | zustand usePlayerStore + 3 tests | `56f13ac` |
| T2 | GET /api/players route + 2 tests | `bfb5702` / `5ab03b2` |
| T3 | GET /api/economy/wallet/[user_id] proxy + economy lib + 2 tests | `5c6e1b3` / `1f1cf42` |
| T4 | GET /api/economy/transactions/[user_id] proxy + 2 tests | `cf7ce48` / `44063a5` |
| T5 | admin emit/sink proxies + 3 tests | `a252771` / `f7f19a6` |
| T6 | PlayerSelector component + 2 tests + error state | `fab470c` / `5c00fa1` |
| T7 | Layout (Sidebar + Header) + 2 tests | `bba46c3` / `5093a5e` |
| T8 | /wallet page with balance cards + 2 tests + refresh UX | `2d9b68c` / `2036f6c` |
| T9 | /transactions page (table + pagination) + 2 tests + 中文 type labels | `f6b92a3` / `e0ec2c0` |
| T10 | /admin page (emit + sink forms) + 2 tests + invalidation | `4d230d1` / `257ab44` |
| T11 | layout.tsx wrap children + dashboard summary | `a610a5c` / `82a7b78` |
| T12 | docker-compose admin-portal env + .env.example | `a68fb34` |
| T13 | Playwright E2E wallet-admin.spec.ts (1 case) | `756d643` |
| T14 | Dockerfile builder corepack + pnpm-lock.yaml | `125dc20` |

**Total: 14 tasks / 24 commits / 92 vitest tests / 1 Playwright E2E**

---

## 验证

### DoD checklist（3.0 v2 admin-portal GA 必过）

- [x] `pnpm test` → 92/92 pass
- [x] `pnpm typecheck` → 0 errors
- [x] `pnpm playwright test e2e/wallet-admin.spec.ts` → 1/1 pass (requires stack)
- [x] `docker compose up -d --build admin-portal` → healthy
- [x] ADR-0009 文档化（本文档）
- [x] CHANGELOG-3.0.md 记录 v2 GA entry
- [x] 3.0-ROADMAP §后续路线 新增 v2 GA section
- [x] admin-portal Dockerfile build 修复（corepack + lockfile）

### 手动验收脚本（stakeholder demo path）

```bash
# 1. 启动 stack
cd /d/work-ai/0401-town/ai-city
docker compose up -d --build

# 2. 打开 admin-portal: http://localhost:8081
# 3. admin / adminpass 登录
# 4. 顶部 PlayerSelector 选 demo → sessionStorage 持久化
# 5. /wallet → 看到 gold=200, token=0（v1 seed 默认值）
# 6. /admin → emit gold=500 to demo → success
# 7. /wallet 刷新 → 看到 gold=700
# 8. /transactions → 看到 1 条 emit 记录（balance_after=700）
```

### 关键 KPI

| KPI | 目标 | 实测 |
|---|---|---|
| Vitest pass | 92/92 | ✅ 92/92 |
| TypeScript errors | 0 | ✅ 0 |
| Playwright E2E | 1/1 pass | ✅ 1/1 |
| Docker container healthy | admin-portal = healthy | ✅ healthy |
| Manual demo path | 5min 内走完 8 步 | ✅ OK |
| Bundle size delta | ≤ 100KB gzip | ✅ +50KB |
| Secrets in browser bundle | 0 | ✅ verified |

### 成功判据（stakeholder 视角）

stakeholder 走完 3.0 v2 demo 时能向同事复述 3 件事：

1. **"我能在 admin 页面看到 demo 的余额，还能手动触发中央银行发钞。"**（UI 闭环）
2. **"交易历史能查、能翻页、按类型着色。"**（表格 + pagination + UX）
3. **"所有 admin 操作不需要找 dev 拿 token，UI 自动转发。"**（proxy pattern + 安全隔离）

如果只能记 1 件 → **proxy pattern**（3.0 v2 最差异化 = 浏览器永远拿不到 ADMIN_TOKEN）。

---

## 项目内文档

- [`docs/3.0-ROADMAP.md §3.0 v2 admin-portal 钱包 UI`](../3.0-ROADMAP.md#30-v2--admin-portal-钱包-ui-ga-2026-10-07) —— v2 GA section
- [`CHANGELOG-3.0.md` v2 GA entry](../../CHANGELOG-3.0.md) —— 新增/修复/文档/测试
- Spec: `docs/superpowers/specs/2026-10-07-admin-portal-wallet-design.md`
- Plan: `docs/superpowers/plans/2026-10-07-admin-portal-wallet.md` —— T1-T14 完整 runbook
- admin-portal README: `apps/admin-portal/README.md` —— 启动 + 测试

### 历史 ADR

- [ADR-0008](0008-3.0-economy-scope.md) —— 3.0 v1 economy-service（本 ADR 前置 + REST API 契约）
- [ADR-0007](0007-2.0-scope-llm-federation-memory-saga.md) —— 2.0 阶段 3 Phase C admin-portal + admin auth（前置）
- [ADR-0006](0006-1.0-scope-mvp-demo-slice.md) —— 1.0 MVP Demo（前置）

### 相关 sprint retro

- 2.0 阶段 3 Phase C.4 retro —— admin auth GA + JWT cookie；v2 复用 server-side decode
- 3.0 v1 GA retro —— economy-service 完成后 stakeholder 反馈"UI 缺失"

---

**维护说明**：

- 本 ADR 在 3.0 v2 GA 时为 **Accepted** 状态
- 3.0 v3（如 WebSocket 实时 / 玩家间转账 UI）需修改本 ADR 状态为 "Superseded by ADR-XXXX"
- 4.0 admin-portal 完整 dashboard（NPC 监控 / observability-agent）需新 ADR
- 本 ADR 影响 admin-portal 25 个新文件 + 4 个修改文件 + 5 个新依赖 + 14 commits 实施决策
