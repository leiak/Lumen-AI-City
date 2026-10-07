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
  Client: vi.fn(() => ({
    query: mockQuery,
    end: mockEnd,
    connect: vi.fn().mockResolvedValue(undefined),
  })),
}));

// Mock auth check
vi.mock('@/lib/auth', () => ({
  isAdmin: vi.fn(() => true),
  decodeToken: vi.fn(() => ({ username: 'admin', role: 'admin' })),
  COOKIE_NAME: 'admin-token',
}));

// Mock next/headers cookies (vitest runs outside Next request scope)
vi.mock('next/headers', () => ({
  cookies: vi.fn(() => ({
    get: vi.fn(() => undefined),
    set: vi.fn(),
  })),
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
    // Verify the SQL query string (defense-in-depth: catches future SQLi or accidental IN-list changes)
    expect(mockQuery).toHaveBeenCalledWith(
      expect.stringContaining("WHERE username IN ('demo', 'admin')")
    );
    // Verify PG client connection lifecycle
    expect(mockEnd).toHaveBeenCalledTimes(1);
  });

  it('returns empty list when DB has no matching users', async () => {
    mockQuery.mockResolvedValueOnce({ rows: [] });
    const req = new Request('http://localhost/api/players');
    const res = await GET(req);
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.players).toEqual([]);
  });

  it('returns 401 when session is not admin', async () => {
    // Override the auth mock for this test
    const { isAdmin } = await import('@/lib/auth');
    (isAdmin as any).mockReturnValueOnce(false);
    const req = new Request('http://localhost/api/players');
    const res = await GET(req);
    expect(res.status).toBe(401);
    const body = await res.json();
    expect(body.error.code).toBe('R_401');
    // DB should not be touched on auth failure
    expect(mockQuery).not.toHaveBeenCalled();
  });
});