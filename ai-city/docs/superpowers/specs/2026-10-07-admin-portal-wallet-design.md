# 3.0 v2 Admin-Portal 钱包 UI — 设计 Spec

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:writing-plans to convert this design into an implementation plan. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **Status:** Approved (2026-10-07)
> **ADR:** TBD (ADR-0009)
> **Roadmap:** `docs/3.0-ROADMAP.md` §后续路线 (v2 候选)

## 目标

在 admin-portal 加钱包 UI，让运营人员可视化查看玩家余额、交易历史，并手动触发中央银行 emit/sink。让 stakeholder 看到 3.0 经济闭环。

## 范围

### 在 v1 范围内
- 3 个新路由：`/wallet` / `/transactions` / `/admin`
- 5 个新 API routes (admin-portal 后端代理 → economy-service)
- Layout 重构：sidebar + 顶部 player_id selector
- zustand store (`usePlayerStore` + sessionStorage persist)
- react-query 数据获取 + manual refresh
- 17 tests (16 unit + 1 e2e)

### 显式不做（v1 YAGNI）
- 玩家间转账 UI（v1 是 read-only + admin actions）
- NPC 商品目录浏览/上架 UI（v2 接入 marketplace）
- WebSocket / SSE 实时事件订阅（v3 candidate）
- 玩家注册 / KYC（已有 a2a-gateway 注册流）
- 移动端适配（admin 仅桌面）
- 多语言货币切换

## 架构

```
┌──────────────────────────────┐  GET /api/players    ┌──────────────────┐
│ admin-portal                 │  GET /api/economy/*  │ economy-service  │
│ (Next.js 15, :8081)          │ ────────────────────►│ (FastAPI, :8005) │
│                              │  POST /api/economy/* │                  │
│ Layout:                      │                      │ PG (source)      │
│ ┌──────────────────────────┐ │  Bearer ADMIN_TOKEN  │ Redis cache      │
│ │ Header: [Player ▾] [👤] │ │ (proxy 加)          │ Kafka events     │
│ ├────────┬─────────────────┤ │                      │                  │
│ │Sidebar │ Wallet Page     │ │                      │                  │
│ │        │ /wallet         │ │                      │                  │
│ │        │ Transactions    │ │                      │                  │
│ │        │ /transactions   │ │                      │                  │
│ │        │ Admin Tools     │ │                      │                  │
│ │        │ /admin          │ │                      │                  │
│ └────────┴─────────────────┘ │                      │                  │
└──────────────────────────────┘                      └──────────────────┘
       ▲
       │ Browser (only sees /api/* paths, never ADMIN_TOKEN)
```

### 关键设计决策

1. **后端代理**：浏览器只调 admin-portal `/api/*`，由 Next.js route handler 转发到 economy-service。`ADMIN_TOKEN` 仅 server-side，浏览器不接触。
2. **Player 列表从 PG 读**：不硬编码。`/api/players` 读 `player` 表 (admin-portal 后端可直连 PG，因为同 docker network)。
3. **Layout 改动最小化**：`layout.tsx` 从 server 改为 client wrapper，但保留 `<html>/<body>` 在 server 文件 (`app/layout.tsx`)。新增 `components/Layout.tsx` 作为 client component。
4. **zustand + sessionStorage**：`usePlayerStore` 跨页持久化 selected player_id。刷新页面或重启浏览器后保留选择。
5. **react-query manual refresh**：v1 不轮询。emit/sink 提交成功后 `queryClient.invalidateQueries(['wallet', playerId])` + `['transactions', playerId]`。
6. **Sidebar nav**：Wallet / Transactions / Admin / (现有) BT Editor / Saga Viz。Dashboard (`/page`) 改为简短 summary。

## API Routes (admin-portal backend)

| Method | Path | Proxies to | Body | Auth |
|---|---|---|---|---|
| GET | `/api/players` | 直查 PG `player` 表 (filter role in (player, admin)) | — | JWT admin |
| GET | `/api/economy/wallet/[user_id]` | `economy-service:8005/api/v1/wallet/{user_id}` | — | JWT admin |
| GET | `/api/economy/transactions/[user_id]?limit=N&offset=M` | `economy-service:8005/api/v1/transactions/{user_id}?limit=N&offset=M` | — | JWT admin |
| POST | `/api/economy/admin/emit` | `economy-service:8005/api/v1/admin/central-bank/emit` (+ `Authorization: Bearer ${ADMIN_TOKEN}`) | — | JWT admin |
| POST | `/api/economy/admin/sink` | 同上 sink endpoint | `{user_id, amount, reason?}` | JWT admin |

**Error mapping**:
- economy-service 返回 4xx/5xx → admin-portal API route 透传 status + body
- Network error / ECONNREFUSED → 502 + `{code: "R_502", msg: "economy-service unreachable"}`
- Missing env (`ADMIN_TOKEN` empty) → 500 + `{code: "R_503", msg: "admin token not configured"}`

## 数据流

1. **登录后访问 /wallet**：
   - 顶部 PlayerSelector 从 `/api/players` 拉列表 (zustand 缓存)
   - 默认选中 sessionStorage 恢复的 `selectedPlayerId`，否则第一个
   - 选完后 zustand 写入 `selectedPlayerId`
   - `<WalletClient>` 用 react-query `['wallet', playerId]` GET `/api/economy/wallet/{id}` 拉余额
   - 显示 2 张卡片 (gold + token) + [刷新] 按钮

2. **/admin emit**：
   - Admin 填 reason → submit
   - react-query mutation → POST `/api/economy/admin/emit` (server 加 Bearer)
   - 200 → toast "Emit 成功 amount=N" + invalidate wallet query
   - 4xx/5xx → toast "失败 code=R_xxx"

3. **/admin sink**：
   - Admin 填 user_id + amount + reason → submit
   - 同 emit 但 body 不空

## UI 详细

### Layout (sidebar + header)

```tsx
// components/Layout.tsx (client)
'use client';
import { useState } from 'react';
import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { PlayerSelector } from './PlayerSelector';
import { Header } from './Header';

const navItems = [
  { href: '/', label: 'Dashboard' },
  { href: '/wallet', label: 'Wallet' },
  { href: '/transactions', label: 'Transactions' },
  { href: '/admin', label: 'Admin Tools' },
  { href: '/bt-editor', label: 'BT Editor' },
  { href: '/saga-viz', label: 'Saga Viz' },
];

export function Layout({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  return (
    <div className="min-h-screen flex flex-col">
      <Header />
      <div className="flex-1 flex">
        <aside className="w-56 bg-white border-r p-4">
          <nav className="space-y-1">
            {navItems.map(item => (
              <Link key={item.href} href={item.href}
                    className={clsx('block px-3 py-2 rounded',
                      pathname === item.href
                        ? 'bg-blue-50 text-blue-700'
                        : 'text-gray-700 hover:bg-gray-100')}>
                {item.label}
              </Link>
            ))}
          </nav>
        </aside>
        <main className="flex-1 p-6">{children}</main>
      </div>
    </div>
  );
}
```

### Wallet page

```tsx
// app/wallet/WalletClient.tsx
'use client';
import { useQuery } from '@tanstack/react-query';
import { usePlayerStore } from '@/lib/usePlayerStore';

export function WalletClient() {
  const playerId = usePlayerStore(s => s.selectedPlayerId);
  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['wallet', playerId],
    queryFn: () => fetch(`/api/economy/wallet/${playerId}`).then(r => r.json()),
    enabled: !!playerId,
  });

  if (!playerId) return <p>请从顶部选择 player。</p>;
  if (isLoading) return <p>加载中…</p>;
  if (error) return <p className="text-red-600">错误: {String(error)}</p>;

  return (
    <div className="grid grid-cols-2 gap-4">
      <BalanceCard label="Gold Balance" value={data.gold_balance} onRefresh={refetch} />
      <BalanceCard label="Token Balance" value={data.token_balance} onRefresh={refetch} />
    </div>
  );
}
```

### Transactions page

```tsx
// app/transactions/TransactionsClient.tsx
'use client';
import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { format } from 'date-fns';

export function TransactionsClient() {
  const playerId = usePlayerStore(s => s.selectedPlayerId);
  const [offset, setOffset] = useState(0);
  const limit = 50;
  const { data, isLoading, refetch } = useQuery({
    queryKey: ['transactions', playerId, limit, offset],
    queryFn: () => fetch(`/api/economy/transactions/${playerId}?limit=${limit}&offset=${offset}`).then(r => r.json()),
    enabled: !!playerId,
  });

  // Table: time | type | currency | amount | balance_after | detail
  // Type badge: player_transfer=blue, npc_purchase=green, central_bank_emit=purple
}
```

### Admin page

```tsx
// app/admin/AdminClient.tsx
'use client';
import { useMutation, useQueryClient } from '@tanstack/react-query';

export function AdminClient() {
  const queryClient = useQueryClient();

  const emitMut = useMutation({
    mutationFn: (reason: string) =>
      fetch('/api/economy/admin/emit', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ reason }),
      }).then(r => r.json()),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['wallet'] });
      queryClient.invalidateQueries({ queryKey: ['transactions'] });
    },
  });

  const sinkMut = useMutation({
    mutationFn: (body: { user_id: string; amount: number; reason?: string }) =>
      fetch('/api/economy/admin/sink', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      }).then(r => r.json()),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['wallet'] });
      queryClient.invalidateQueries({ queryKey: ['transactions'] });
    },
  });

  // Two forms: emit + sink, each with input + submit button + result display
}
```

## 状态管理

### zustand store

```typescript
// lib/usePlayerStore.ts
import { create } from 'zustand';
import { persist } from 'zustand/middleware';

interface PlayerState {
  selectedPlayerId: string | null;
  setSelected: (id: string) => void;
  clear: () => void;
}

export const usePlayerStore = create<PlayerState>()(
  persist(
    (set) => ({
      selectedPlayerId: null,
      setSelected: (id) => set({ selectedPlayerId: id }),
      clear: () => set({ selectedPlayerId: null }),
    }),
    { name: 'admin-portal-player' }  // sessionStorage key
  )
);
```

## 部署

### docker-compose.yml 改动 (admin-portal service)

```yaml
admin-portal:
  environment:
    ADMIN_PORTAL_ECONOMY_URL: "http://economy-service:8005"
    ADMIN_PORTAL_ADMIN_TOKEN: "${ADMIN_TOKEN:-dev-admin-token}"
    DATABASE_URL: "postgresql://aicity:aicity_dev@postgres:5432/aicity"  # for /api/players
```

### .env.example (admin-portal)

```
ADMIN_PORTAL_JWT_SECRET=dev-secret-change-me
ADMIN_PORTAL_ECONOMY_URL=http://economy-service:8005
ADMIN_PORTAL_ADMIN_TOKEN=dev-admin-token
```

## 测试策略

### Unit tests (vitest + RTL, 16 tests)

| File | Tests |
|---|---|
| `tests/api/proxy.test.ts` | (1) GET wallet proxy → fetch called with right URL + headers |
|                          | (2) GET transactions proxy → limit/offset forwarded |
|                          | (3) POST emit proxy → Bearer token added from env |
|                          | (4) POST sink proxy → body + Bearer token |
| `tests/api/players.test.ts` | (1) Returns demo + admin players only |
|                              | (2) Empty list when DB has no matching users |
| `tests/lib/usePlayerStore.test.ts` | (1) setSelected updates state |
|                                       | (2) persist middleware writes sessionStorage |
| `tests/wallet.test.tsx` | (1) Renders balance cards on data |
|                         | (2) Shows "请选择 player" when none selected |
| `tests/transactions.test.tsx` | (1) Renders history table |
|                                  | (2) Pagination buttons update offset |
| `tests/admin.test.tsx` | (1) emit mutation success shows toast + invalidates queries |
|                         | (2) sink form validates amount > 0 |
| `tests/layout.test.tsx` | (1) Sidebar renders 6 nav items |
|                           | (2) Active link gets highlight class |

### E2E (Playwright, 1 test)

`tests/e2e/wallet-admin.spec.ts`:
1. Login as admin/adminpass
2. Navigate to /wallet → see player selector → click demo
3. Verify gold + token cards display numbers
4. Navigate to /admin → fill emit reason → click "触发 emit"
5. Verify success toast or error code displayed
6. Navigate back to /wallet → click 刷新 → balance updated

## 关键文件 (15+)

### Create
- `apps/admin-portal/src/components/Layout.tsx`
- `apps/admin-portal/src/components/Sidebar.tsx`
- `apps/admin-portal/src/components/Header.tsx`
- `apps/admin-portal/src/components/PlayerSelector.tsx`
- `apps/admin-portal/src/lib/usePlayerStore.ts`
- `apps/admin-portal/src/lib/economy.ts` (typed fetch helpers + error mapping)
- `apps/admin-portal/src/app/wallet/page.tsx` (server auth check)
- `apps/admin-portal/src/app/wallet/WalletClient.tsx`
- `apps/admin-portal/src/app/transactions/page.tsx`
- `apps/admin-portal/src/app/transactions/TransactionsClient.tsx`
- `apps/admin-portal/src/app/admin/page.tsx`
- `apps/admin-portal/src/app/admin/AdminClient.tsx`
- `apps/admin-portal/src/app/api/players/route.ts`
- `apps/admin-portal/src/app/api/economy/wallet/[user_id]/route.ts`
- `apps/admin-portal/src/app/api/economy/transactions/[user_id]/route.ts`
- `apps/admin-portal/src/app/api/economy/admin/emit/route.ts`
- `apps/admin-portal/src/app/api/economy/admin/sink/route.ts`
- `apps/admin-portal/tests/api/proxy.test.ts` (4 tests)
- `apps/admin-portal/tests/api/players.test.ts` (2 tests)
- `apps/admin-portal/tests/lib/usePlayerStore.test.ts` (2 tests)
- `apps/admin-portal/tests/wallet.test.tsx` (2 tests)
- `apps/admin-portal/tests/transactions.test.tsx` (2 tests)
- `apps/admin-portal/tests/admin.test.tsx` (2 tests)
- `apps/admin-portal/tests/layout.test.tsx` (2 tests)
- `apps/admin-portal/e2e/wallet-admin.spec.ts` (1 case)

### Modify
- `apps/admin-portal/src/app/layout.tsx` (wrap children in `<Layout>`)
- `apps/admin-portal/src/app/page.tsx` (改为 dashboard summary cards)
- `docker-compose.yml` (admin-portal env vars)
- `apps/admin-portal/.env.example` (add ADMIN_PORTAL_* envs)

### Read (existing, no modify)
- `apps/admin-portal/src/lib/auth.ts` (existing JWT helpers)
- `apps/admin-portal/src/app/bt-editor/` (existing client component pattern)
- `apps/admin-portal/src/app/saga-viz/` (existing client component pattern)
- `apps/admin-portal/src/app/login/page.tsx` (auth pattern)

## 风险

| 风险 | 缓解 |
|---|---|
| Layout 重构破坏现有 /bt-editor / /saga-viz | Playwright e2e 1 case 跑通现有路由；visual regression 验证 |
| ADMIN_TOKEN 错配导致 admin actions 全 403 | 启动时 .env validation 启动失败而非运行时 403；启动日志 print "ADMIN_TOKEN configured: True/False" |
| player_id 输入错 UUID 导致后续 API 404 | Playwright e2e 用 demo player_id；UI 禁用按钮直到 valid |
| react-query cache 在 SSR 泄漏 | query key 含 playerId；client-side only 用 `enabled: !!playerId` |
| economy-service 不可达 → admin portal 502 | proxy route 返回结构化 502 + R_502 错误码；前端 toast 友好提示 |
| Layout 改 client 后 metadata 等 server-only API 失效 | 把 metadata 留在 server layout.tsx；client Layout 只包 children |

## Critical Files (pre-implementation)

详见 §关键文件 列表。**重点先读**:
- `apps/admin-portal/src/lib/auth.ts` (JWT helpers)
- `apps/admin-portal/src/app/bt-editor/BtEditorClient.tsx` (client component 模式)
- `apps/admin-portal/src/app/saga-viz/page.tsx` (page.tsx 模式)
- `apps/economy-service/src/economy_service/api/v1/wallet.py` (response shape)
- `apps/economy-service/src/economy_service/api/v1/transactions.py` (response shape)
- `apps/economy-service/src/economy_service/api/v1/admin.py` (SinkRequest schema)

## Acceptance (DoD)

1. `pnpm test` → 16 unit tests pass
2. `pnpm playwright test e2e/wallet-admin.spec.ts` → 1 case pass
3. `docker compose up -d --build` → admin-portal healthy
4. 手动验证 (浏览器):
   - 登录 admin → 看到 sidebar 6 项
   - 选 demo player → 钱包余额显示
   - 查看 transactions 表格 5+ 条
   - 在 admin 页触发 emit → 看到 success toast
   - 返回 wallet 点刷新 → 余额变化
5. `pnpm typecheck` → 0 errors
6. `pnpm build` → 0 errors
