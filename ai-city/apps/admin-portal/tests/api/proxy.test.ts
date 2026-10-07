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