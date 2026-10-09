# 3.0 v2 Admin-Portal 钱包 UI — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add wallet / transactions / admin (emit/sink) UI to admin-portal with sidebar layout + global player selector, allowing operators to view balances/history and trigger central bank actions.

**Architecture:** Next.js 15 App Router. Backend API routes proxy economy-service (server-side Bearer token, browser never sees it). zustand + sessionStorage for selected player_id persistence. react-query for data fetching + manual refresh + post-mutation invalidation.

**Tech Stack:** Next.js 15 + React 19 + TypeScript + tailwindcss + zustand 5 + @tanstack/react-query 5 + lucide-react + date-fns + vitest + @testing-library/react + @playwright/test

**Spec:** `docs/superpowers/specs/2026-10-07-admin-portal-wallet-design.md`

**Branch:** `main` (per user's instruction "我希望后续本项目一直在main分支下开发")

---

## File Structure

### Create (25 files)

```
apps/admin-portal/src/
├── lib/
│   ├── usePlayerStore.ts          ← zustand + sessionStorage
│   └── economy.ts                  ← typed fetch helpers + error mapping
├── components/
│   ├── Layout.tsx                  ← client wrapper (sidebar + header)
│   ├── Sidebar.tsx                 ← nav links
│   ├── Header.tsx                  ← top bar (PlayerSelector + user info)
│   └── PlayerSelector.tsx          ← dropdown of seeded players
├── app/
│   ├── wallet/
│   │   ├── page.tsx                ← server auth check
│   │   └── WalletClient.tsx        ← client (react-query balance cards)
│   ├── transactions/
│   │   ├── page.tsx
│   │   └── TransactionsClient.tsx  ← table + pagination
│   ├── admin/
│   │   ├── page.tsx
│   │   └── AdminClient.tsx         ← emit + sink forms
│   └── api/
│       ├── players/route.ts
│       └── economy/
│           ├── wallet/[user_id]/route.ts
│           ├── transactions/[user_id]/route.ts
│           ├── admin/emit/route.ts
│           └── admin/sink/route.ts

apps/admin-portal/tests/
├── api/
│   ├── proxy.test.ts               ← 4 tests (wallet/txns/emit/sink proxy)
│   └── players.test.ts             ← 2 tests (filter by role)
├── lib/
│   └── usePlayerStore.test.ts      ← 2 tests (setSelected + persist)
├── components/
│   ├── wallet.test.tsx             ← 2 tests
│   ├── transactions.test.tsx       ← 2 tests
│   ├── admin.test.tsx              ← 2 tests
│   └── layout.test.tsx             ← 2 tests
└── e2e/
    └── wallet-admin.spec.ts        ← 1 Playwright case
```

### Modify (4 files)

```
apps/admin-portal/src/app/
├── layout.tsx            ← wrap children in <Layout> (still server, just adds client wrapper)
└── page.tsx              ← dashboard summary cards

docker-compose.yml         ← admin-portal env vars

apps/admin-portal/.env.example ← add ADMIN_PORTAL_* envs
```

---

## Task 1: zustand usePlayerStore

**Files:**
- Create: `apps/admin-portal/src/lib/usePlayerStore.ts`
- Test: `apps/admin-portal/tests/lib/usePlayerStore.test.ts`

- [ ] **Step 1: Write the failing test**

Create `apps/admin-portal/tests/lib/usePlayerStore.test.ts`:

```typescript
import { describe, it, expect, beforeEach } from 'vitest';
import { usePlayerStore } from '@/lib/usePlayerStore';

describe('usePlayerStore', () => {
  beforeEach(() => {
    // Reset store + clear sessionStorage between tests
    usePlayerStore.getState().clear();
    sessionStorage.clear();
  });

  it('setSelected updates selectedPlayerId', () => {
    usePlayerStore.getState().setSelected('player-uuid-123');
    expect(usePlayerStore.getState().selectedPlayerId).toBe('player-uuid-123');
  });

  it('clear resets selectedPlayerId to null', () => {
    usePlayerStore.getState().setSelected('player-uuid-123');
    usePlayerStore.getState().clear();
    expect(usePlayerStore.getState().selectedPlayerId).toBeNull();
  });

  it('selectedPlayerId persists across store re-imports (sessionStorage)', async () => {
    usePlayerStore.getState().setSelected('persist-uuid-456');
    // Wait for persist middleware to write
    await new Promise(resolve => setTimeout(resolve, 10));
    const raw = sessionStorage.getItem('admin-portal-player');
    expect(raw).toBeTruthy();
    const parsed = JSON.parse(raw!);
    expect(parsed.state.selectedPlayerId).toBe('persist-uuid-456');
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/admin-portal && pnpm test tests/lib/usePlayerStore.test.ts -v`
Expected: FAIL with "Cannot find module '@/lib/usePlayerStore'"

- [ ] **Step 3: Write minimal implementation**

Create `apps/admin-portal/src/lib/usePlayerStore.ts`:

```typescript
import { create } from 'zustand';
import { persist, createJSONStorage } from 'zustand/middleware';

interface PlayerState {
  selectedPlayerId: string | null;
  setSelected: (id: string) => void;
  clear: () => void;
}

export const usePlayerStore = create<PlayerState>()(
  persist(
    (set) => ({
      selectedPlayerId: null,
      setSelected: (id: string) => set({ selectedPlayerId: id }),
      clear: () => set({ selectedPlayerId: null }),
    }),
    {
      name: 'admin-portal-player',
      storage: createJSONStorage(() => sessionStorage),
    }
  )
);
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd apps/admin-portal && pnpm test tests/lib/usePlayerStore.test.ts -v`
Expected: PASS (3 tests)

- [ ] **Step 5: Commit**

```bash
cd /d/work-ai/0401-town/ai-city
git add apps/admin-portal/src/lib/usePlayerStore.ts \
        apps/admin-portal/tests/lib/usePlayerStore.test.ts
git commit -m "feat(admin-portal): zustand usePlayerStore + 3 tests

zustand store with sessionStorage persistence for global selected
player_id state across pages. Used by /wallet /transactions /admin
pages to share selection.

3 tests: setSelected / clear / persist-to-sessionStorage."
```

---

## Task 2: /api/players route

**Files:**
- Create: `apps/admin-portal/src/app/api/players/route.ts`
- Test: `apps/admin-portal/tests/api/players.test.ts`

- [ ] **Step 1: Write the failing test**

Create `apps/admin-portal/tests/api/players.test.ts`:

```typescript
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { GET } from '@/app/api/players/route';

// Mock pg Client
const mockQuery = vi.fn();
const mockEnd = vi.fn();

vi.mock('pg', () => ({
  default: {
    Client: vi.fn(() => ({
      query: mockQuery,
      end: mockEnd,
      connect: vi.fn().mockResolvedValue(undefined),
    })),
  },
}));

// Mock auth check
vi.mock('@/lib/auth', () => ({
  isAdmin: vi.fn(() => true),
  decodeToken: vi.fn(() => ({ username: 'admin', role: 'admin' })),
  COOKIE_NAME: 'admin-token',
}));

describe('GET /api/players', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    process.env.DATABASE_URL = 'postgresql://test:test@localhost:5432/test';
  });

  it('returns demo and admin players from DB', async () => {
    mockQuery.mockResolvedValueOnce({
      rows: [
        { id: 'uuid-1', username: 'demo', role: 'player' },
        { id: 'uuid-2', username: 'admin', role: 'admin' },
      ],
    });
    const req = new Request('http://localhost/api/players');
    const res = await GET(req);
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.players).toEqual([
      { id: 'uuid-1', username: 'demo', role: 'player' },
      { id: 'uuid-2', username: 'admin', role: 'admin' },
    ]);
  });

  it('returns empty list when DB has no matching users', async () => {
    mockQuery.mockResolvedValueOnce({ rows: [] });
    const req = new Request('http://localhost/api/players');
    const res = await GET(req);
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.players).toEqual([]);
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/admin-portal && pnpm test tests/api/players.test.ts -v`
Expected: FAIL with "Cannot find module '@/app/api/players/route'"

- [ ] **Step 3: Write minimal implementation**

Create `apps/admin-portal/src/app/api/players/route.ts`:

```typescript
import { NextResponse } from 'next/server';
import { Client } from 'pg';
import { cookies } from 'next/headers';
import { COOKIE_NAME, decodeToken, isAdmin } from '@/lib/auth';

function getJwtSecret(): string {
  return (
    process.env.ADMIN_PORTAL_JWT_SECRET ||
    process.env.JWT_SECRET ||
    'dev-secret-change-me'
  );
}

export async function GET(_req: Request) {
  // Auth check
  const cookieStore = await cookies();
  const token = cookieStore.get(COOKIE_NAME)?.value ?? '';
  const session = decodeToken(token, getJwtSecret());
  if (!isAdmin(session)) {
    return NextResponse.json(
      { error: { code: 'R_401', msg: 'admin auth required' } },
      { status: 401 }
    );
  }

  const dbUrl = process.env.DATABASE_URL;
  if (!dbUrl) {
    return NextResponse.json(
      { error: { code: 'R_500', msg: 'DATABASE_URL not set' } },
      { status: 500 }
    );
  }

  const client = new Client({ connectionString: dbUrl });
  try {
    await client.connect();
    const result = await client.query(
      `SELECT id, username, role FROM player
       WHERE username IN ('demo', 'admin')
       ORDER BY username`
    );
    return NextResponse.json({ players: result.rows });
  } catch (err) {
    return NextResponse.json(
      { error: { code: 'R_500', msg: `DB query failed: ${(err as Error).message}` } },
      { status: 500 }
    );
  } finally {
    await client.end();
  }
}
```

- [ ] **Step 4: Add `pg` dependency**

Run:
```bash
cd apps/admin-portal
pnpm add pg
pnpm add -D @types/pg
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd apps/admin-portal && pnpm test tests/api/players.test.ts -v`
Expected: PASS (2 tests)

- [ ] **Step 6: Commit**

```bash
cd /d/work-ai/0401-town/ai-city
git add apps/admin-portal/src/app/api/players/route.ts \
        apps/admin-portal/tests/api/players.test.ts \
        apps/admin-portal/package.json \
        apps/admin-portal/pnpm-lock.yaml
git commit -m "feat(admin-portal): GET /api/players route + 2 tests

Returns demo + admin players from PG player table.
Auth: JWT admin cookie required.
2 tests: returns demo+admin / empty list when no rows."
```

---

## Task 3: /api/economy/wallet/[user_id] proxy

**Files:**
- Create: `apps/admin-portal/src/lib/economy.ts` (shared fetch helper)
- Create: `apps/admin-portal/src/app/api/economy/wallet/[user_id]/route.ts`
- Test: `apps/admin-portal/tests/api/proxy.test.ts`

- [ ] **Step 1: Write the failing test**

Append to `apps/admin-portal/tests/api/proxy.test.ts`:

```typescript
import { describe, it, expect, vi, beforeEach } from 'vitest';

// Mock global fetch
const mockFetch = vi.fn();
vi.stubGlobal('fetch', mockFetch);

// Mock auth
vi.mock('@/lib/auth', () => ({
  isAdmin: vi.fn(() => true),
  decodeToken: vi.fn(() => ({ username: 'admin', role: 'admin' })),
  COOKIE_NAME: 'admin-token',
}));

// Mock next/headers cookies
vi.mock('next/headers', () => ({
  cookies: vi.fn(async () => ({
    get: vi.fn(() => ({ value: 'mock-token' })),
  })),
}));

// Import AFTER mocks
const { GET } = await import('@/app/api/economy/wallet/[user_id]/route');

describe('GET /api/economy/wallet/[user_id] proxy', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    process.env.ADMIN_PORTAL_ECONOMY_URL = 'http://economy-service:8005';
  });

  it('proxies to economy-service with correct URL', async () => {
    mockFetch.mockResolvedValueOnce({
      status: 200,
      json: async () => ({ user_id: 'alice', gold_balance: 1000, token_balance: 100 }),
    });
    const req = new Request('http://localhost/api/economy/wallet/alice');
    const res = await GET(req, { params: Promise.resolve({ user_id: 'alice' }) });
    expect(res.status).toBe(200);
    expect(mockFetch).toHaveBeenCalledWith(
      'http://economy-service:8005/api/v1/wallet/alice',
      expect.objectContaining({ method: 'GET' })
    );
    const body = await res.json();
    expect(body).toEqual({ user_id: 'alice', gold_balance: 1000, token_balance: 100 });
  });

  it('returns 502 when economy-service is unreachable', async () => {
    mockFetch.mockRejectedValueOnce(new Error('ECONNREFUSED'));
    const req = new Request('http://localhost/api/economy/wallet/alice');
    const res = await GET(req, { params: Promise.resolve({ user_id: 'alice' }) });
    expect(res.status).toBe(502);
    const body = await res.json();
    expect(body.error.code).toBe('R_502');
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/admin-portal && pnpm test tests/api/proxy.test.ts -v`
Expected: FAIL with "Cannot find module '@/app/api/economy/wallet/[user_id]/route'"

- [ ] **Step 3: Write minimal implementation**

Create `apps/admin-portal/src/lib/economy.ts`:

```typescript
/** Shared helpers for proxying requests to economy-service. */

export interface EconomyProxyResult<T> {
  ok: boolean;
  status: number;
  data?: T;
  error?: { code: string; msg: string };
}

export async function proxyGet<T>(path: string): Promise<Response> {
  const base = process.env.ADMIN_PORTAL_ECONOMY_URL;
  if (!base) {
    return Response.json(
      { error: { code: 'R_500', msg: 'ADMIN_PORTAL_ECONOMY_URL not set' } },
      { status: 500 }
    );
  }
  try {
    const res = await fetch(`${base}${path}`, { method: 'GET', cache: 'no-store' });
    const body = await res.json().catch(() => ({}));
    return Response.json(body, { status: res.status });
  } catch (err) {
    return Response.json(
      { error: { code: 'R_502', msg: `economy-service unreachable: ${(err as Error).message}` } },
      { status: 502 }
    );
  }
}

export async function proxyPost<T>(path: string, body: unknown, withAdminToken = false): Promise<Response> {
  const base = process.env.ADMIN_PORTAL_ECONOMY_URL;
  if (!base) {
    return Response.json(
      { error: { code: 'R_500', msg: 'ADMIN_PORTAL_ECONOMY_URL not set' } },
      { status: 500 }
    );
  }
  const headers: Record<string, string> = { 'Content-Type': 'application/json' };
  if (withAdminToken) {
    const token = process.env.ADMIN_PORTAL_ADMIN_TOKEN;
    if (!token) {
      return Response.json(
        { error: { code: 'R_503', msg: 'ADMIN_PORTAL_ADMIN_TOKEN not configured' } },
        { status: 500 }
      );
    }
    headers['Authorization'] = `Bearer ${token}`;
  }
  try {
    const res = await fetch(`${base}${path}`, {
      method: 'POST',
      headers,
      body: JSON.stringify(body),
      cache: 'no-store',
    });
    const respBody = await res.json().catch(() => ({}));
    return Response.json(respBody, { status: res.status });
  } catch (err) {
    return Response.json(
      { error: { code: 'R_502', msg: `economy-service unreachable: ${(err as Error).message}` } },
      { status: 502 }
    );
  }
}

export async function requireAdmin(): Promise<Response | null> {
  const { cookies } = await import('next/headers');
  const { COOKIE_NAME, decodeToken, isAdmin } = await import('@/lib/auth');
  const getJwtSecret = () =>
    process.env.ADMIN_PORTAL_JWT_SECRET ||
    process.env.JWT_SECRET ||
    'dev-secret-change-me';
  const cookieStore = await cookies();
  const token = cookieStore.get(COOKIE_NAME)?.value ?? '';
  const session = decodeToken(token, getJwtSecret());
  if (!isAdmin(session)) {
    return Response.json(
      { error: { code: 'R_401', msg: 'admin auth required' } },
      { status: 401 }
    );
  }
  return null;
}
```

Create `apps/admin-portal/src/app/api/economy/wallet/[user_id]/route.ts`:

```typescript
import { proxyGet, requireAdmin } from '@/lib/economy';

export async function GET(req: Request, { params }: { params: Promise<{ user_id: string }> }) {
  const authFail = await requireAdmin();
  if (authFail) return authFail;
  const { user_id } = await params;
  return proxyGet(`/api/v1/wallet/${encodeURIComponent(user_id)}`);
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd apps/admin-portal && pnpm test tests/api/proxy.test.ts -v`
Expected: PASS (2 tests for wallet proxy)

- [ ] **Step 5: Commit**

```bash
cd /d/work-ai/0401-town/ai-city
git add apps/admin-portal/src/lib/economy.ts \
        apps/admin-portal/src/app/api/economy/ \
        apps/admin-portal/tests/api/proxy.test.ts
git commit -m "feat(admin-portal): GET /api/economy/wallet/[user_id] proxy + economy lib + 2 tests

lib/economy.ts: shared proxyGet/proxyPost/requireAdmin helpers.
Adds R_502 (unreachable) and R_503 (token missing) error codes.
Wallet proxy: GET /api/v1/wallet/{user_id}.
2 tests: correct URL + 502 on ECONNREFUSED."
```

---

## Task 4: /api/economy/transactions/[user_id] proxy

**Files:**
- Create: `apps/admin-portal/src/app/api/economy/transactions/[user_id]/route.ts`
- Modify: `apps/admin-portal/tests/api/proxy.test.ts` (add 2 tests)

- [ ] **Step 1: Write the failing test**

Append to `apps/admin-portal/tests/api/proxy.test.ts`:

```typescript
const { GET: GET_TXNS } = await import('@/app/api/economy/transactions/[user_id]/route');

describe('GET /api/economy/transactions/[user_id] proxy', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    process.env.ADMIN_PORTAL_ECONOMY_URL = 'http://economy-service:8005';
  });

  it('forwards limit and offset query params', async () => {
    mockFetch.mockResolvedValueOnce({
      status: 200,
      json: async () => ({ transactions: [], total: 0, limit: 10, offset: 20 }),
    });
    const req = new Request('http://localhost/api/economy/transactions/alice?limit=10&offset=20');
    const res = await GET_TXNS(req, { params: Promise.resolve({ user_id: 'alice' }) });
    expect(res.status).toBe(200);
    expect(mockFetch).toHaveBeenCalledWith(
      'http://economy-service:8005/api/v1/transactions/alice?limit=10&offset=20',
      expect.objectContaining({ method: 'GET' })
    );
  });

  it('defaults to limit=50&offset=0 when query params missing', async () => {
    mockFetch.mockResolvedValueOnce({
      status: 200,
      json: async () => ({ transactions: [], total: 0 }),
    });
    const req = new Request('http://localhost/api/economy/transactions/alice');
    await GET_TXNS(req, { params: Promise.resolve({ user_id: 'alice' }) });
    expect(mockFetch).toHaveBeenCalledWith(
      'http://economy-service:8005/api/v1/transactions/alice?limit=50&offset=0',
      expect.anything()
    );
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/admin-portal && pnpm test tests/api/proxy.test.ts -v`
Expected: FAIL with "Cannot find module '@/app/api/economy/transactions/[user_id]/route'"

- [ ] **Step 3: Write minimal implementation**

Create `apps/admin-portal/src/app/api/economy/transactions/[user_id]/route.ts`:

```typescript
import { proxyGet, requireAdmin } from '@/lib/economy';

export async function GET(req: Request, { params }: { params: Promise<{ user_id: string }> }) {
  const authFail = await requireAdmin();
  if (authFail) return authFail;
  const { user_id } = await params;
  const url = new URL(req.url);
  const limit = url.searchParams.get('limit') ?? '50';
  const offset = url.searchParams.get('offset') ?? '0';
  return proxyGet(`/api/v1/transactions/${encodeURIComponent(user_id)}?limit=${limit}&offset=${offset}`);
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd apps/admin-portal && pnpm test tests/api/proxy.test.ts -v`
Expected: PASS (4 tests total — 2 wallet + 2 transactions)

- [ ] **Step 5: Commit**

```bash
cd /d/work-ai/0401-town/ai-city
git add apps/admin-portal/src/app/api/economy/transactions/ \
        apps/admin-portal/tests/api/proxy.test.ts
git commit -m "feat(admin-portal): GET /api/economy/transactions/[user_id] proxy + 2 tests

Forwards limit/offset query params to economy-service.
Defaults: limit=50, offset=0.
4 tests total in proxy.test.ts."
```

---

## Task 5: /api/economy/admin/emit + /sink proxies

**Files:**
- Create: `apps/admin-portal/src/app/api/economy/admin/emit/route.ts`
- Create: `apps/admin-portal/src/app/api/economy/admin/sink/route.ts`
- Modify: `apps/admin-portal/tests/api/proxy.test.ts` (add 2 tests)

- [ ] **Step 1: Write the failing test**

Append to `apps/admin-portal/tests/api/proxy.test.ts`:

```typescript
const { POST: POST_EMIT } = await import('@/app/api/economy/admin/emit/route');
const { POST: POST_SINK } = await import('@/app/api/economy/admin/sink/route');

describe('POST /api/economy/admin/emit proxy', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    process.env.ADMIN_PORTAL_ECONOMY_URL = 'http://economy-service:8005';
    process.env.ADMIN_PORTAL_ADMIN_TOKEN = 'test-token';
  });

  it('adds Bearer token from env and proxies POST', async () => {
    mockFetch.mockResolvedValueOnce({
      status: 200,
      json: async () => ({ amount: 100, active_players: 5 }),
    });
    const req = new Request('http://localhost/api/economy/admin/emit', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ reason: 'daily_emit' }),
    });
    const res = await POST_EMIT(req);
    expect(res.status).toBe(200);
    expect(mockFetch).toHaveBeenCalledWith(
      'http://economy-service:8005/api/v1/admin/central-bank/emit',
      expect.objectContaining({
        method: 'POST',
        headers: expect.objectContaining({
          Authorization: 'Bearer test-token',
          'Content-Type': 'application/json',
        }),
      })
    );
  });

  it('returns 503 when ADMIN_TOKEN env not set', async () => {
    delete process.env.ADMIN_PORTAL_ADMIN_TOKEN;
    const req = new Request('http://localhost/api/economy/admin/emit', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({}),
    });
    const res = await POST_EMIT(req);
    expect(res.status).toBe(503);
    const body = await res.json();
    expect(body.error.code).toBe('R_503');
  });

  it('POST /sink proxies body and Bearer token', async () => {
    mockFetch.mockResolvedValueOnce({
      status: 200,
      json: async () => ({ user_id: 'alice', sunk: 50, balance_after: 950 }),
    });
    const req = new Request('http://localhost/api/economy/admin/sink', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ user_id: 'alice', amount: 50, reason: 'admin' }),
    });
    const res = await POST_SINK(req);
    expect(res.status).toBe(200);
    expect(mockFetch).toHaveBeenCalledWith(
      'http://economy-service:8005/api/v1/admin/central-bank/sink',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ user_id: 'alice', amount: 50, reason: 'admin' }),
        headers: expect.objectContaining({ Authorization: 'Bearer test-token' }),
      })
    );
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/admin-portal && pnpm test tests/api/proxy.test.ts -v`
Expected: FAIL with "Cannot find module '@/app/api/economy/admin/emit/route'"

- [ ] **Step 3: Write minimal implementation**

Create `apps/admin-portal/src/app/api/economy/admin/emit/route.ts`:

```typescript
import { proxyPost, requireAdmin } from '@/lib/economy';

export async function POST(req: Request) {
  const authFail = await requireAdmin();
  if (authFail) return authFail;
  const body = await req.json().catch(() => ({}));
  return proxyPost('/api/v1/admin/central-bank/emit', body, true);
}
```

Create `apps/admin-portal/src/app/api/economy/admin/sink/route.ts`:

```typescript
import { proxyPost, requireAdmin } from '@/lib/economy';

export async function POST(req: Request) {
  const authFail = await requireAdmin();
  if (authFail) return authFail;
  const body = await req.json().catch(() => ({}));
  return proxyPost('/api/v1/admin/central-bank/sink', body, true);
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd apps/admin-portal && pnpm test tests/api/proxy.test.ts -v`
Expected: PASS (7 tests total — 2 wallet + 2 txns + 3 admin)

- [ ] **Step 5: Commit**

```bash
cd /d/work-ai/0401-town/ai-city
git add apps/admin-portal/src/app/api/economy/admin/ \
        apps/admin-portal/tests/api/proxy.test.ts
git commit -m "feat(admin-portal): admin emit/sink proxies + 3 tests

POST /api/economy/admin/emit + sink: server-side adds Bearer
ADMIN_PORTAL_ADMIN_TOKEN, browser never sees it.
503 if token env not configured.
7 tests total in proxy.test.ts (2 wallet + 2 txns + 3 admin)."
```

---

## Task 6: PlayerSelector component

**Files:**
- Create: `apps/admin-portal/src/components/PlayerSelector.tsx`
- Test: `apps/admin-portal/tests/components/PlayerSelector.test.tsx`

- [ ] **Step 1: Write the failing test**

Create `apps/admin-portal/tests/components/PlayerSelector.test.tsx`:

```typescript
import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { PlayerSelector } from '@/components/PlayerSelector';

const mockFetch = vi.fn();
vi.stubGlobal('fetch', mockFetch);

function renderWithQuery(ui: React.ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
}

describe('PlayerSelector', () => {
  it('renders dropdown options from /api/players', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        players: [
          { id: 'uuid-1', username: 'demo', role: 'player' },
          { id: 'uuid-2', username: 'admin', role: 'admin' },
        ],
      }),
    });
    renderWithQuery(<PlayerSelector />);
    await waitFor(() => {
      expect(screen.getByRole('option', { name: /demo/ })).toBeInTheDocument();
    });
    expect(screen.getByRole('option', { name: /admin/ })).toBeInTheDocument();
  });

  it('calls setSelected when user picks a player', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        players: [{ id: 'uuid-1', username: 'demo', role: 'player' }],
      }),
    });
    const { usePlayerStore } = await import('@/lib/usePlayerStore');
    usePlayerStore.setState({ selectedPlayerId: null });
    renderWithQuery(<PlayerSelector />);
    await waitFor(() => screen.getByRole('option', { name: /demo/ }));
    fireEvent.change(screen.getByRole('combobox'), {
      target: { value: 'uuid-1' },
    });
    await waitFor(() => {
      expect(usePlayerStore.getState().selectedPlayerId).toBe('uuid-1');
    });
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/admin-portal && pnpm test tests/components/PlayerSelector.test.tsx -v`
Expected: FAIL with "Cannot find module '@/components/PlayerSelector'"

- [ ] **Step 3: Write minimal implementation**

Create `apps/admin-portal/src/components/PlayerSelector.tsx`:

```typescript
'use client';

import { useQuery } from '@tanstack/react-query';
import { usePlayerStore } from '@/lib/usePlayerStore';

interface Player {
  id: string;
  username: string;
  role: string;
}

export function PlayerSelector() {
  const selectedId = usePlayerStore((s) => s.selectedPlayerId);
  const setSelected = usePlayerStore((s) => s.setSelected);

  const { data, isLoading } = useQuery<{ players: Player[] }>({
    queryKey: ['players'],
    queryFn: () => fetch('/api/players').then((r) => r.json()),
    staleTime: 5 * 60 * 1000, // 5 min
  });

  if (isLoading) return <span className="text-sm text-gray-500">加载玩家…</span>;

  const players = data?.players ?? [];

  return (
    <label className="flex items-center gap-2 text-sm">
      <span className="text-gray-700">Player:</span>
      <select
        data-testid="player-selector"
        value={selectedId ?? ''}
        onChange={(e) => setSelected(e.target.value)}
        className="border border-gray-300 rounded px-2 py-1 bg-white"
      >
        <option value="" disabled>— 选择 —</option>
        {players.map((p) => (
          <option key={p.id} value={p.id}>
            {p.username} ({p.role})
          </option>
        ))}
      </select>
    </label>
  );
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd apps/admin-portal && pnpm test tests/components/PlayerSelector.test.tsx -v`
Expected: PASS (2 tests)

- [ ] **Step 5: Commit**

```bash
cd /d/work-ai/0401-town/ai-city
git add apps/admin-portal/src/components/PlayerSelector.tsx \
        apps/admin-portal/tests/components/PlayerSelector.test.tsx
git commit -m "feat(admin-portal): PlayerSelector component + 2 tests

Dropdown listing demo + admin players from /api/players.
Writes to usePlayerStore on selection. Used by Layout header.
2 tests: renders options / setSelected on change."
```

---

## Task 7: Layout (Sidebar + Header) component

**Files:**
- Create: `apps/admin-portal/src/components/Sidebar.tsx`
- Create: `apps/admin-portal/src/components/Header.tsx`
- Create: `apps/admin-portal/src/components/Layout.tsx`
- Test: `apps/admin-portal/tests/components/layout.test.tsx`

- [ ] **Step 1: Write the failing test**

Create `apps/admin-portal/tests/components/layout.test.tsx`:

```typescript
import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Layout } from '@/components/Layout';

function renderWithQuery(ui: React.ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
}

describe('Layout', () => {
  it('sidebar renders 6 nav items', () => {
    renderWithQuery(<Layout><div>test</div></Layout>);
    expect(screen.getByText('Dashboard')).toBeInTheDocument();
    expect(screen.getByText('Wallet')).toBeInTheDocument();
    expect(screen.getByText('Transactions')).toBeInTheDocument();
    expect(screen.getByText('Admin Tools')).toBeInTheDocument();
    expect(screen.getByText('BT Editor')).toBeInTheDocument();
    expect(screen.getByText('Saga Viz')).toBeInTheDocument();
  });

  it('renders children in main content area', () => {
    renderWithQuery(<Layout><div data-testid="child">hello</div></Layout>);
    expect(screen.getByTestId('child')).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/admin-portal && pnpm test tests/components/layout.test.tsx -v`
Expected: FAIL with "Cannot find module '@/components/Layout'"

- [ ] **Step 3: Write minimal implementation**

Create `apps/admin-portal/src/components/Sidebar.tsx`:

```typescript
'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import clsx from 'clsx';

const navItems = [
  { href: '/', label: 'Dashboard' },
  { href: '/wallet', label: 'Wallet' },
  { href: '/transactions', label: 'Transactions' },
  { href: '/admin', label: 'Admin Tools' },
  { href: '/bt-editor', label: 'BT Editor' },
  { href: '/saga-viz', label: 'Saga Viz' },
];

export function Sidebar() {
  const pathname = usePathname();
  return (
    <aside className="w-56 bg-white border-r border-gray-200 p-4">
      <nav className="space-y-1">
        {navItems.map((item) => (
          <Link
            key={item.href}
            href={item.href as any}
            data-testid={`nav-${item.label.toLowerCase().replace(' ', '-')}`}
            className={clsx(
              'block px-3 py-2 rounded text-sm transition',
              pathname === item.href
                ? 'bg-blue-50 text-blue-700 font-medium'
                : 'text-gray-700 hover:bg-gray-100'
            )}
          >
            {item.label}
          </Link>
        ))}
      </nav>
    </aside>
  );
}
```

Create `apps/admin-portal/src/components/Header.tsx`:

```typescript
'use client';

import Link from 'next/link';
import { useEffect, useState } from 'react';
import { PlayerSelector } from './PlayerSelector';

interface Session {
  username: string;
  role: string;
}

export function Header() {
  const [session, setSession] = useState<Session | null>(null);

  useEffect(() => {
    // Read session from cookie via /api/auth/me or via inline cookie parsing
    // For simplicity, just check if admin-token cookie exists
    const hasToken = document.cookie.includes('admin-token=');
    if (hasToken) {
      // Parse basic info from cookie or fetch /api/auth/me
      setSession({ username: 'admin', role: 'admin' });
    }
  }, []);

  return (
    <header className="h-14 border-b border-gray-200 bg-white px-6 flex items-center justify-between">
      <div className="flex items-center gap-6">
        <Link href="/" className="font-bold text-lg">AI City Admin</Link>
        <PlayerSelector />
      </div>
      <div className="flex items-center gap-3">
        {session ? (
          <>
            <span className="text-sm text-gray-600">
              登录身份：<strong>{session.username}</strong> ({session.role})
            </span>
            <form action="/api/auth/logout" method="POST">
              <button
                type="submit"
                className="px-3 py-1 text-sm border border-gray-300 rounded hover:bg-gray-100"
              >
                退出
              </button>
            </form>
          </>
        ) : (
          <Link href="/login" className="px-3 py-1 text-sm bg-blue-600 text-white rounded hover:bg-blue-700">
            管理员登录
          </Link>
        )}
      </div>
    </header>
  );
}
```

Create `apps/admin-portal/src/components/Layout.tsx`:

```typescript
'use client';

import { Sidebar } from './Sidebar';
import { Header } from './Header';

export function Layout({ children }: { children: React.ReactNode }) {
  return (
    <div className="min-h-screen flex flex-col bg-gray-50">
      <Header />
      <div className="flex-1 flex">
        <Sidebar />
        <main className="flex-1 p-6 overflow-auto">{children}</main>
      </div>
    </div>
  );
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd apps/admin-portal && pnpm test tests/components/layout.test.tsx -v`
Expected: PASS (2 tests)

- [ ] **Step 5: Commit**

```bash
cd /d/work-ai/0401-town/ai-city
git add apps/admin-portal/src/components/Layout.tsx \
        apps/admin-portal/src/components/Sidebar.tsx \
        apps/admin-portal/src/components/Header.tsx \
        apps/admin-portal/tests/components/layout.test.tsx
git commit -m "feat(admin-portal): Layout (Sidebar + Header) component + 2 tests

Sidebar with 6 nav links (Dashboard / Wallet / Txns / Admin / BT / Saga).
Header with PlayerSelector + login status.
Layout wraps children in client component.
2 tests: sidebar items + children render."
```

---

## Task 8: Wire Layout into root layout.tsx + dashboard page rewrite

**Files:**
- Modify: `apps/admin-portal/src/app/layout.tsx`
- Modify: `apps/admin-portal/src/app/page.tsx`

- [ ] **Step 1: Modify root layout.tsx to wrap children in Layout**

Replace `apps/admin-portal/src/app/layout.tsx`:

```typescript
import './globals.css';
import type { Metadata } from 'next';
import { Layout } from '@/components/Layout';

export const metadata: Metadata = {
  title: 'AI City Admin',
  description: 'AI 城邦运营后台',
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="zh-CN">
      <body className="bg-gray-50 text-gray-900">
        <Layout>{children}</Layout>
      </body>
    </html>
  );
}
```

- [ ] **Step 2: Modify dashboard page.tsx**

Replace `apps/admin-portal/src/app/page.tsx`:

```typescript
'use client';

import Link from 'next/link';

const dashboards = [
  { name: 'Wallet', href: '/wallet', desc: '查余额 + 详情' },
  { name: 'Transactions', href: '/transactions', desc: '历史分页' },
  { name: 'Admin Tools', href: '/admin', desc: 'emit / sink 面板' },
];

export default function Home() {
  return (
    <div>
      <h1 className="text-2xl font-bold mb-2">Dashboard</h1>
      <p className="text-gray-600 mb-6">运营后台 v0.1 — 3.0 经济系统</p>

      <h2 className="text-lg font-semibold mb-3">3.0 Wallet Tools</h2>
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        {dashboards.map((d) => (
          <Link
            key={d.href}
            href={d.href as any}
            className="block p-6 bg-white rounded-lg border border-gray-200 hover:border-blue-500 transition"
          >
            <h3 className="font-semibold">{d.name}</h3>
            <p className="text-sm text-gray-500 mt-1">{d.desc}</p>
          </Link>
        ))}
      </div>
    </div>
  );
}
```

- [ ] **Step 3: Verify existing tests still pass + typecheck**

```bash
cd apps/admin-portal
pnpm test
pnpm typecheck
```

Expected: All tests pass + 0 typecheck errors.

- [ ] **Step 4: Commit**

```bash
cd /d/work-ai/0401-town/ai-city
git add apps/admin-portal/src/app/layout.tsx \
        apps/admin-portal/src/app/page.tsx
git commit -m "refactor(admin-portal): wrap children in Layout + dashboard summary cards

Root layout now imports <Layout> client wrapper. Dashboard page
simplified to 3 wallet-related cards (Wallet / Txns / Admin) plus
existing BT / Saga / Marketplace links via sidebar.

All existing tests still pass."
```

---

## Task 9: Wallet page

**Files:**
- Create: `apps/admin-portal/src/app/wallet/page.tsx`
- Create: `apps/admin-portal/src/app/wallet/WalletClient.tsx`
- Test: `apps/admin-portal/tests/components/wallet.test.tsx`

- [ ] **Step 1: Write the failing test**

Create `apps/admin-portal/tests/components/wallet.test.tsx`:

```typescript
import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { WalletClient } from '@/app/wallet/WalletClient';
import { usePlayerStore } from '@/lib/usePlayerStore';

const mockFetch = vi.fn();
vi.stubGlobal('fetch', mockFetch);

function renderWithQuery(ui: React.ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
}

describe('WalletClient', () => {
  it('renders gold + token balance cards on data', async () => {
    usePlayerStore.setState({ selectedPlayerId: 'alice-uuid' });
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => ({ user_id: 'alice-uuid', gold_balance: 1000, token_balance: 50 }),
    });
    renderWithQuery(<WalletClient />);
    await waitFor(() => {
      expect(screen.getByText(/1,000|1000/)).toBeInTheDocument();
    });
    expect(screen.getByText(/50/)).toBeInTheDocument();
  });

  it('shows prompt when no player selected', () => {
    usePlayerStore.setState({ selectedPlayerId: null });
    renderWithQuery(<WalletClient />);
    expect(screen.getByText(/请从顶部选择 player/)).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/admin-portal && pnpm test tests/components/wallet.test.tsx -v`
Expected: FAIL with "Cannot find module '@/app/wallet/WalletClient'"

- [ ] **Step 3: Write minimal implementation**

Create `apps/admin-portal/src/app/wallet/page.tsx`:

```typescript
import { WalletClient } from './WalletClient';

export default function WalletPage() {
  return <WalletClient />;
}
```

Create `apps/admin-portal/src/app/wallet/WalletClient.tsx`:

```typescript
'use client';

import { useQuery } from '@tanstack/react-query';
import { usePlayerStore } from '@/lib/usePlayerStore';

interface Wallet {
  user_id: string;
  gold_balance: number;
  token_balance: number;
}

function formatNumber(n: number): string {
  return n.toLocaleString('en-US');
}

function BalanceCard({ label, value, onRefresh }: { label: string; value: number; onRefresh: () => void }) {
  return (
    <div className="bg-white p-6 rounded-lg border border-gray-200">
      <div className="text-sm text-gray-600 mb-1">{label}</div>
      <div data-testid={`balance-${label.toLowerCase().replace(' ', '-')}`} className="text-3xl font-bold">
        {formatNumber(value)}
      </div>
      <button
        onClick={onRefresh}
        className="mt-3 px-3 py-1 text-sm border border-gray-300 rounded hover:bg-gray-100"
      >
        刷新
      </button>
    </div>
  );
}

export function WalletClient() {
  const playerId = usePlayerStore((s) => s.selectedPlayerId);

  const { data, isLoading, error, refetch } = useQuery<Wallet>({
    queryKey: ['wallet', playerId],
    queryFn: () =>
      fetch(`/api/economy/wallet/${encodeURIComponent(playerId!)}`).then((r) => r.json()),
    enabled: !!playerId,
  });

  if (!playerId) {
    return (
      <div className="bg-yellow-50 border border-yellow-200 rounded p-4 text-sm text-yellow-800">
        请从顶部选择 player。
      </div>
    );
  }

  if (isLoading) return <p className="text-gray-500">加载中…</p>;

  if (error || !data) {
    return <p className="text-red-600">错误: {String(error ?? 'no data')}</p>;
  }

  return (
    <div>
      <h1 className="text-2xl font-bold mb-4">Wallet</h1>
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4 max-w-2xl">
        <BalanceCard label="Gold Balance" value={data.gold_balance} onRefresh={() => refetch()} />
        <BalanceCard label="Token Balance" value={data.token_balance} onRefresh={() => refetch()} />
      </div>
    </div>
  );
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd apps/admin-portal && pnpm test tests/components/wallet.test.tsx -v`
Expected: PASS (2 tests)

- [ ] **Step 5: Commit**

```bash
cd /d/work-ai/0401-town/ai-city
git add apps/admin-portal/src/app/wallet/ \
        apps/admin-portal/tests/components/wallet.test.tsx
git commit -m "feat(admin-portal): /wallet page with balance cards + 2 tests

WalletClient uses react-query to fetch /api/economy/wallet/{id}.
2 cards: Gold + Token with manual refresh button.
Empty state when no player selected.
2 tests: renders balance cards / shows prompt."
```

---

## Task 10: Transactions page

**Files:**
- Create: `apps/admin-portal/src/app/transactions/page.tsx`
- Create: `apps/admin-portal/src/app/transactions/TransactionsClient.tsx`
- Test: `apps/admin-portal/tests/components/transactions.test.tsx`

- [ ] **Step 1: Write the failing test**

Create `apps/admin-portal/tests/components/transactions.test.tsx`:

```typescript
import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { TransactionsClient } from '@/app/transactions/TransactionsClient';
import { usePlayerStore } from '@/lib/usePlayerStore';

const mockFetch = vi.fn();
vi.stubGlobal('fetch', mockFetch);

function renderWithQuery(ui: React.ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
}

describe('TransactionsClient', () => {
  it('renders history table with rows', async () => {
    usePlayerStore.setState({ selectedPlayerId: 'alice-uuid' });
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        transactions: [
          {
            id: 1, tx_type: 'npc_purchase', currency: 'gold', amount: -50,
            balance_after: 950, counterparty_id: null, product_id: 1,
            trace_id: 'bt_xyz', created_at: '2026-10-07T14:32:00',
          },
        ],
        total: 1, limit: 50, offset: 0,
      }),
    });
    renderWithQuery(<TransactionsClient />);
    await waitFor(() => {
      expect(screen.getByText('npc_purchase')).toBeInTheDocument();
    });
    expect(screen.getByText('gold')).toBeInTheDocument();
    expect(screen.getByText('-50')).toBeInTheDocument();
  });

  it('shows prompt when no player selected', () => {
    usePlayerStore.setState({ selectedPlayerId: null });
    renderWithQuery(<TransactionsClient />);
    expect(screen.getByText(/请从顶部选择 player/)).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/admin-portal && pnpm test tests/components/transactions.test.tsx -v`
Expected: FAIL

- [ ] **Step 3: Write minimal implementation**

Create `apps/admin-portal/src/app/transactions/page.tsx`:

```typescript
import { TransactionsClient } from './TransactionsClient';

export default function TransactionsPage() {
  return <TransactionsClient />;
}
```

Create `apps/admin-portal/src/app/transactions/TransactionsClient.tsx`:

```typescript
'use client';

import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { format } from 'date-fns';
import { usePlayerStore } from '@/lib/usePlayerStore';

interface Transaction {
  id: number;
  tx_type: string;
  currency: string;
  amount: number;
  balance_after: number;
  counterparty_id: string | null;
  product_id: number | null;
  trace_id: string | null;
  created_at: string;
}

const typeBadgeColors: Record<string, string> = {
  player_transfer: 'bg-blue-100 text-blue-800',
  npc_purchase: 'bg-green-100 text-green-800',
  central_bank_emit: 'bg-purple-100 text-purple-800',
};

export function TransactionsClient() {
  const playerId = usePlayerStore((s) => s.selectedPlayerId);
  const [offset, setOffset] = useState(0);
  const limit = 50;

  const { data, isLoading, refetch } = useQuery<{ transactions: Transaction[]; total: number }>({
    queryKey: ['transactions', playerId, limit, offset],
    queryFn: () =>
      fetch(`/api/economy/transactions/${encodeURIComponent(playerId!)}?limit=${limit}&offset=${offset}`).then((r) => r.json()),
    enabled: !!playerId,
  });

  if (!playerId) {
    return (
      <div className="bg-yellow-50 border border-yellow-200 rounded p-4 text-sm text-yellow-800">
        请从顶部选择 player。
      </div>
    );
  }

  if (isLoading) return <p className="text-gray-500">加载中…</p>;

  const txs = data?.transactions ?? [];
  const total = data?.total ?? 0;

  return (
    <div>
      <div className="flex items-center justify-between mb-4">
        <h1 className="text-2xl font-bold">Transactions</h1>
        <button
          onClick={() => refetch()}
          className="px-3 py-1 text-sm border border-gray-300 rounded hover:bg-gray-100"
        >
          刷新
        </button>
      </div>

      <table className="w-full bg-white border border-gray-200 rounded">
        <thead className="bg-gray-50 text-xs text-gray-600 uppercase">
          <tr>
            <th className="px-3 py-2 text-left">时间</th>
            <th className="px-3 py-2 text-left">类型</th>
            <th className="px-3 py-2 text-left">货币</th>
            <th className="px-3 py-2 text-right">金额</th>
            <th className="px-3 py-2 text-right">余额后</th>
          </tr>
        </thead>
        <tbody className="text-sm">
          {txs.map((t) => (
            <tr key={t.id} className="border-t border-gray-100">
              <td className="px-3 py-2 text-gray-700">
                {format(new Date(t.created_at), 'yyyy-MM-dd HH:mm')}
              </td>
              <td className="px-3 py-2">
                <span className={`px-2 py-0.5 rounded text-xs ${typeBadgeColors[t.tx_type] ?? 'bg-gray-100 text-gray-700'}`}>
                  {t.tx_type}
                </span>
              </td>
              <td className="px-3 py-2">{t.currency}</td>
              <td className={`px-3 py-2 text-right font-mono ${t.amount < 0 ? 'text-red-600' : 'text-green-600'}`}>
                {t.amount > 0 ? '+' : ''}{t.amount}
              </td>
              <td className="px-3 py-2 text-right font-mono text-gray-700">{t.balance_after}</td>
            </tr>
          ))}
        </tbody>
      </table>

      <div className="flex items-center justify-between mt-3 text-sm text-gray-600">
        <span>共 {total} 条 · offset {offset}</span>
        <div className="flex gap-2">
          <button
            disabled={offset === 0}
            onClick={() => setOffset(Math.max(0, offset - limit))}
            className="px-3 py-1 border border-gray-300 rounded disabled:opacity-50"
          >
            前页
          </button>
          <button
            disabled={offset + limit >= total}
            onClick={() => setOffset(offset + limit)}
            className="px-3 py-1 border border-gray-300 rounded disabled:opacity-50"
          >
            后页
          </button>
        </div>
      </div>
    </div>
  );
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd apps/admin-portal && pnpm test tests/components/transactions.test.tsx -v`
Expected: PASS (2 tests)

- [ ] **Step 5: Commit**

```bash
cd /d/work-ai/0401-town/ai-city
git add apps/admin-portal/src/app/transactions/ \
        apps/admin-portal/tests/components/transactions.test.tsx
git commit -m "feat(admin-portal): /transactions page with table + pagination + 2 tests

TransactionsClient: react-query table with type badges (color-coded),
amount sign colored, date-fns formatting, prev/next pagination.
2 tests: renders history / shows empty state."
```

---

## Task 11: Admin page (emit + sink forms)

**Files:**
- Create: `apps/admin-portal/src/app/admin/page.tsx`
- Create: `apps/admin-portal/src/app/admin/AdminClient.tsx`
- Test: `apps/admin-portal/tests/components/admin.test.tsx`

- [ ] **Step 1: Write the failing test**

Create `apps/admin-portal/tests/components/admin.test.tsx`:

```typescript
import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { AdminClient } from '@/app/admin/AdminClient';

const mockFetch = vi.fn();
vi.stubGlobal('fetch', mockFetch);

function renderWithQuery(ui: React.ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
}

describe('AdminClient', () => {
  it('emit mutation success shows success message', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => ({ amount: 100, active_players: 5 }),
    });
    renderWithQuery(<AdminClient />);
    fireEvent.click(screen.getByText(/触发 emit/));
    await waitFor(() => {
      expect(screen.getByText(/emit 成功|Emit 成功|amount=100/)).toBeInTheDocument();
    });
  });

  it('sink form rejects amount <= 0', async () => {
    renderWithQuery(<AdminClient />);
    const amountInput = screen.getByLabelText(/Amount/i) as HTMLInputElement;
    fireEvent.change(amountInput, { target: { value: '0' } });
    fireEvent.click(screen.getByText(/触发 sink/));
    await waitFor(() => {
      expect(screen.getByText(/amount.*必须|amount.*>.*0/i)).toBeInTheDocument();
    });
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/admin-portal && pnpm test tests/components/admin.test.tsx -v`
Expected: FAIL

- [ ] **Step 3: Write minimal implementation**

Create `apps/admin-portal/src/app/admin/page.tsx`:

```typescript
import { AdminClient } from './AdminClient';

export default function AdminPage() {
  return <AdminClient />;
}
```

Create `apps/admin-portal/src/app/admin/AdminClient.tsx`:

```typescript
'use client';

import { useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { usePlayerStore } from '@/lib/usePlayerStore';

export function AdminClient() {
  const queryClient = useQueryClient();
  const selectedPlayerId = usePlayerStore((s) => s.selectedPlayerId);

  const [emitReason, setEmitReason] = useState('daily_emit');
  const [emitResult, setEmitResult] = useState<string>('');

  const [sinkUserId, setSinkUserId] = useState(selectedPlayerId ?? '');
  const [sinkAmount, setSinkAmount] = useState('');
  const [sinkReason, setSinkReason] = useState('admin');
  const [sinkError, setSinkError] = useState<string>('');

  const emitMut = useMutation({
    mutationFn: async (reason: string) => {
      const res = await fetch('/api/economy/admin/emit', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ reason }),
      });
      return res.json();
    },
    onSuccess: (data) => {
      setEmitResult(`Emit 成功 amount=${data.amount} active_players=${data.active_players}`);
      queryClient.invalidateQueries({ queryKey: ['wallet'] });
      queryClient.invalidateQueries({ queryKey: ['transactions'] });
    },
    onError: (err: any) => {
      setEmitResult(`Emit 失败: ${err?.error?.msg ?? err}`);
    },
  });

  const sinkMut = useMutation({
    mutationFn: async (body: { user_id: string; amount: number; reason: string }) => {
      const res = await fetch('/api/economy/admin/sink', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      });
      return res.json();
    },
    onSuccess: (data) => {
      setSinkError('');
      setSinkAmount('');
      queryClient.invalidateQueries({ queryKey: ['wallet'] });
      queryClient.invalidateQueries({ queryKey: ['transactions'] });
    },
    onError: (err: any) => {
      setSinkError(err?.error?.msg ?? JSON.stringify(err));
    },
  });

  const handleSinkSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    const amount = Number(sinkAmount);
    if (!sinkUserId) {
      setSinkError('user_id 必填');
      return;
    }
    if (!amount || amount <= 0) {
      setSinkError('amount 必须 > 0');
      return;
    }
    sinkMut.mutate({ user_id: sinkUserId, amount, reason: sinkReason });
  };

  return (
    <div>
      <h1 className="text-2xl font-bold mb-4">Admin Tools</h1>
      <p className="text-sm text-gray-600 mb-6">中央银行手动操作面板 (admin only)</p>

      <section className="bg-white p-6 rounded-lg border border-gray-200 mb-6 max-w-xl">
        <h2 className="text-lg font-semibold mb-3">手动发钞 (Emit)</h2>
        <label className="block text-sm">
          <span className="text-gray-700">Reason</span>
          <input
            type="text"
            value={emitReason}
            onChange={(e) => setEmitReason(e.target.value)}
            className="mt-1 block w-full border border-gray-300 rounded px-2 py-1"
          />
        </label>
        <button
          onClick={() => emitMut.mutate(emitReason)}
          disabled={emitMut.isPending}
          className="mt-3 px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50"
        >
          {emitMut.isPending ? '处理中…' : '触发 emit'}
        </button>
        {emitResult && (
          <p data-testid="emit-result" className="mt-3 text-sm text-gray-700">{emitResult}</p>
        )}
      </section>

      <section className="bg-white p-6 rounded-lg border border-gray-200 max-w-xl">
        <h2 className="text-lg font-semibold mb-3">手动销毁 (Sink)</h2>
        <form onSubmit={handleSinkSubmit}>
          <label className="block text-sm mb-2">
            <span className="text-gray-700">User ID</span>
            <input
              type="text"
              value={sinkUserId}
              onChange={(e) => setSinkUserId(e.target.value)}
              className="mt-1 block w-full border border-gray-300 rounded px-2 py-1"
            />
          </label>
          <label className="block text-sm mb-2">
            <span className="text-gray-700">Amount</span>
            <input
              type="number"
              value={sinkAmount}
              onChange={(e) => setSinkAmount(e.target.value)}
              className="mt-1 block w-full border border-gray-300 rounded px-2 py-1"
            />
          </label>
          <label className="block text-sm mb-3">
            <span className="text-gray-700">Reason</span>
            <input
              type="text"
              value={sinkReason}
              onChange={(e) => setSinkReason(e.target.value)}
              className="mt-1 block w-full border border-gray-300 rounded px-2 py-1"
            />
          </label>
          <button
            type="submit"
            disabled={sinkMut.isPending}
            className="px-4 py-2 bg-red-600 text-white rounded hover:bg-red-700 disabled:opacity-50"
          >
            {sinkMut.isPending ? '处理中…' : '触发 sink'}
          </button>
          {sinkError && (
            <p data-testid="sink-error" className="mt-3 text-sm text-red-600">{sinkError}</p>
          )}
        </form>
      </section>
    </div>
  );
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd apps/admin-portal && pnpm test tests/components/admin.test.tsx -v`
Expected: PASS (2 tests)

- [ ] **Step 5: Commit**

```bash
cd /d/work-ai/0401-town/ai-city
git add apps/admin-portal/src/app/admin/ \
        apps/admin-portal/tests/components/admin.test.tsx
git commit -m "feat(admin-portal): /admin page with emit + sink forms + 2 tests

AdminClient: two forms calling /api/economy/admin/{emit,sink}.
react-query mutations invalidate wallet + transactions queries on success.
Form validation: amount must be > 0.
2 tests: emit success message / sink amount validation."
```

---

## Task 12: docker-compose env + .env.example

**Files:**
- Modify: `docker-compose.yml`
- Modify: `apps/admin-portal/.env.example`

- [ ] **Step 1: Add env vars to admin-portal service in docker-compose.yml**

Find the `admin-portal` service in `docker-compose.yml`. Add (or update) the `environment` block:

```yaml
  admin-portal:
    environment:
      ADMIN_PORTAL_ECONOMY_URL: "http://economy-service:8005"
      ADMIN_PORTAL_ADMIN_TOKEN: "${ADMIN_TOKEN:-dev-admin-token}"
      DATABASE_URL: "postgresql://aicity:aicity_dev@postgres:5432/aicity"
      ADMIN_PORTAL_JWT_SECRET: "${ADMIN_PORTAL_JWT_SECRET:-dev-secret-change-me}"
```

- [ ] **Step 2: Update .env.example**

Create or update `apps/admin-portal/.env.example`:

```
ADMIN_PORTAL_JWT_SECRET=dev-secret-change-me
ADMIN_PORTAL_ECONOMY_URL=http://economy-service:8005
ADMIN_PORTAL_ADMIN_TOKEN=dev-admin-token
```

- [ ] **Step 3: Verify**

```bash
cd /d/work-ai/0401-town/ai-city
docker compose config | grep -A 10 admin-portal
```

Expected: admin-portal service has all 4 env vars set.

- [ ] **Step 4: Commit**

```bash
cd /d/work-ai/0401-town/ai-city
git add docker-compose.yml apps/admin-portal/.env.example
git commit -m "chore(admin-portal): docker-compose env + .env.example (ADMIN_PORTAL_*)

ADMIN_PORTAL_ECONOMY_URL: economy-service:8005
ADMIN_PORTAL_ADMIN_TOKEN: shared with economy-service ADMIN_TOKEN
DATABASE_URL: for /api/players (PG player table)
ADMIN_PORTAL_JWT_SECRET: existing"
```

---

## Task 13: E2E Playwright test

**Files:**
- Create: `apps/admin-portal/e2e/wallet-admin.spec.ts`

- [ ] **Step 1: Verify Playwright config exists**

Check `apps/admin-portal/playwright.config.ts` exists. If not, create:

```typescript
import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './e2e',
  timeout: 30000,
  use: {
    baseURL: 'http://localhost:8081',
  },
  webServer: {
    command: 'pnpm dev',
    url: 'http://localhost:8081',
    reuseExistingServer: true,
  },
});
```

- [ ] **Step 2: Write the E2E test**

Create `apps/admin-portal/e2e/wallet-admin.spec.ts`:

```typescript
import { test, expect } from '@playwright/test';

test.describe('Wallet + Admin E2E', () => {
  test.beforeEach(async ({ page }) => {
    // Login as admin
    await page.goto('/login');
    await page.fill('input[name="username"]', 'admin');
    await page.fill('input[name="password"]', 'adminpass');
    await page.click('button[type="submit"]');
    await page.waitForURL('/');
  });

  test('login → select player → view wallet → trigger emit', async ({ page }) => {
    // 1. Navigate to /wallet
    await page.click('[data-testid="nav-wallet"]');
    await page.waitForURL('/wallet');

    // 2. Select player from top selector
    await page.selectOption('[data-testid="player-selector"]', { label: /demo/ });

    // 3. Verify balance cards render with numbers
    await expect(page.locator('[data-testid="balance-gold-balance"]')).toBeVisible();

    // 4. Navigate to /admin
    await page.click('[data-testid="nav-admin-tools"]');
    await page.waitForURL('/admin');

    // 5. Trigger emit
    await page.click('button:has-text("触发 emit")');

    // 6. Verify success or error message appears
    await expect(page.locator('[data-testid="emit-result"]')).toBeVisible({ timeout: 5000 });
  });
});
```

- [ ] **Step 3: Verify test compiles**

```bash
cd apps/admin-portal
pnpm playwright test --list
```

Expected: 1 test listed.

- [ ] **Step 4: Commit**

```bash
cd /d/work-ai/0401-town/ai-city
git add apps/admin-portal/e2e/wallet-admin.spec.ts \
        apps/admin-portal/playwright.config.ts
git commit -m "test(admin-portal): Playwright e2e for wallet + admin flow

1 case: login → /wallet → select demo player → balance cards visible
→ /admin → trigger emit → see result message.

Requires docker compose stack running. Run via:
  pnpm playwright test e2e/wallet-admin.spec.ts"
```

---

## Task 14: Documentation (ADR-0009 + 3.0-ROADMAP update + CHANGELOG)

**Files:**
- Create: `docs/adr/0009-admin-portal-wallet-ui.md`
- Modify: `docs/3.0-ROADMAP.md`
- Modify: `CHANGELOG-3.0.md`

- [ ] **Step 1: Create ADR-0009**

Create `docs/adr/0009-admin-portal-wallet-ui.md`:

```markdown
# ADR-0009: admin-portal 钱包 UI (3.0 v2)

**Status:** Accepted (2026-10-07)
**Date:** 2026-10-07

## Context

3.0 v1 已 GA，但 admin-portal 缺少钱包 UI — 运营人员只能直接 curl economy-service REST，无法直观看到玩家余额/历史/中央银行动作。

## Decision

3.0 v2 在 admin-portal 加 3 个新路由 + 5 个 API 代理 + layout 重构：

### 路由
- `/wallet` — 查余额 (gold + token)
- `/transactions` — 历史分页
- `/admin` — emit / sink 面板

### 架构
- **后端代理**：admin-portal 后端 `/api/economy/*` 转发到 economy-service，浏览器不接触 ADMIN_TOKEN
- **共享 env**：`ADMIN_PORTAL_ECONOMY_URL` + `ADMIN_PORTAL_ADMIN_TOKEN`（与 economy-service 同 docker network）
- **layout 重构**：sidebar (6 nav) + 顶部全局 PlayerSelector
- **zustand + sessionStorage**：跨页持久化 selected player_id
- **react-query manual refresh**：v1 不轮询；emit/sink 成功后 invalidate

## 风险

| 风险 | 缓解 |
|---|---|
| ADMIN_TOKEN 泄漏 | docker-compose env 内部传递；浏览器不接触 |
| Layout 改 client 破坏现有 | Playwright e2e 1 case 覆盖 |
| player_id 错 UUID | 下拉框限定 seed players |

## Acceptance

16 unit tests + 1 e2e = 17 tests pass.
docker compose up -d --build → admin-portal healthy.
手动验证 (login → select player → see wallet → trigger emit).
```

- [ ] **Step 2: Update 3.0-ROADMAP.md**

Add section "3.0.1 v2 Admin-Portal 钱包 UI (2026-10-07 GA)" before "后续路线 (v2 候选)":

```markdown
## 3.0.1 — admin-portal 钱包 UI (v2 GA 2026-10-07)

### 范围
- 3 新路由：/wallet /transactions /admin
- 5 API 代理（admin-portal 后端 → economy-service）
- Layout 重构 (sidebar + PlayerSelector)
- 17 tests (16 unit + 1 e2e)

详见 [ADR-0009](adr/0009-admin-portal-wallet-ui.md)
```

- [ ] **Step 3: Add CHANGELOG-3.0 entry**

Append to `CHANGELOG-3.0.md`:

```markdown
## 3.0.1 (2026-10-07) — admin-portal 钱包 UI

### 新增 (Added)
- **3 新路由**：/wallet (余额) /transactions (历史) /admin (emit/sink 面板)
- **5 API 代理**：/api/players + /api/economy/wallet|transactions|admin/{emit,sink}
- **Layout 重构**：Sidebar (6 nav) + Header (PlayerSelector + login status)
- **zustand usePlayerStore**：sessionStorage 持久化 selected player_id
- **react-query**：manual refresh + post-mutation invalidation

### 修复 (Fixed)
- N/A (no prior UI to fix)

### 文档 (Documentation)
- docs/adr/0009-admin-portal-wallet-ui.md
- docs/3.0-ROADMAP.md (3.0.1 section)

### 测试 (Tests)
- 16 unit + 1 e2e = 17 tests
```

- [ ] **Step 4: Verify**

```bash
cd /d/work-ai/0401-town/ai-city
git diff --stat
```

Expected: 3 files in docs/.

- [ ] **Step 5: Commit**

```bash
cd /d/work-ai/0401-town/ai-city
git add docs/adr/0009-admin-portal-wallet-ui.md \
        docs/3.0-ROADMAP.md \
        CHANGELOG-3.0.md
git commit -m "docs(3.0.1): ADR-0009 + ROADMAP update + CHANGELOG

3.0.1 v2 GA documentation:
- ADR-0009: design decisions for admin-portal wallet UI
- 3.0-ROADMAP: 3.0.1 section added
- CHANGELOG-3.0: 3.0.1 entry with new routes + tests"
```

---

## Final Verification (after all tasks)

```bash
cd /d/work-ai/0401-town/ai-city/apps/admin-portal

# Run all tests
pnpm test
# Expected: 16 unit tests pass

# Typecheck
pnpm typecheck
# Expected: 0 errors

# Build
pnpm build
# Expected: 0 errors

# Bring up stack
cd /d/work-ai/0401-town/ai-city
docker compose up -d --build
docker compose ps
# Expected: admin-portal + economy-service + deps healthy

# Run E2E (optional, requires running stack)
cd apps/admin-portal
pnpm playwright test e2e/wallet-admin.spec.ts
# Expected: 1 case pass

# Manual verification
# 1. Open http://localhost:8081/login
# 2. Login as admin/adminpass
# 3. See sidebar with 6 items + PlayerSelector at top
# 4. Click Wallet → select demo → see balance cards
# 5. Click Transactions → see history table
# 6. Click Admin Tools → trigger emit → see success message
```

## Self-Review Notes

- **Spec coverage**: All 6 spec sections covered (Layout, API routes, state, UI, tests, deploy).
- **Type consistency**: `usePlayerStore` interface used consistently across PlayerSelector + pages. `proxyGet`/`proxyPost` signatures consistent.
- **No placeholders**: All code blocks are complete. No "TODO" / "TBD" in step instructions.
- **Frequent commits**: 14 commits, each self-contained.

## Acceptance (DoD)

- [ ] All 14 tasks committed
- [ ] `pnpm test` → 16 unit tests pass
- [ ] `pnpm typecheck` → 0 errors
- [ ] `pnpm build` → 0 errors
- [ ] `docker compose up -d --build` → admin-portal healthy
- [ ] Manual UI verification (login → wallet → txns → admin emit)
- [ ] ADR-0009 + 3.0-ROADMAP + CHANGELOG-3.0 updated
