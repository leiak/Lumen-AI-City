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

  it('returns 401 when session is not admin', async () => {
    const { isAdmin } = await import('@/lib/auth');
    (isAdmin as any).mockReturnValueOnce(false);
    const req = new Request('http://localhost/api/economy/admin/emit', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({}),
    });
    const res = await POST_EMIT(req);
    expect(res.status).toBe(401);
    const body = await res.json();
    expect(body.error.code).toBe('R_401');
    // economy-service must not be touched on auth failure
    expect(mockFetch).not.toHaveBeenCalled();
  });
});

describe('POST /api/economy/admin/sink proxy', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    process.env.ADMIN_PORTAL_ECONOMY_URL = 'http://economy-service:8005';
    process.env.ADMIN_PORTAL_ADMIN_TOKEN = 'test-token';
  });

  it('proxies body and Bearer token', async () => {
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

  it('forwards malformed JSON as empty body (current contract)', async () => {
    // Documents the existing .catch(() => ({})) behavior: the routes accept
    // malformed JSON silently and forward {} upstream. If a future change
    // wants strict 400 parsing, this test will fail and force a contract update.
    mockFetch.mockResolvedValueOnce({
      status: 200,
      json: async () => ({ ok: true }),
    });
    const req = new Request('http://localhost/api/economy/admin/sink', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: 'not-json',
    });
    await POST_SINK(req);
    expect(mockFetch).toHaveBeenCalledWith(
      expect.any(String),
      expect.objectContaining({ body: '{}' })
    );
  });
});