import { beforeEach, describe, expect, it, vi } from 'vitest';

type MockSession = {
  username: string;
  role: 'admin' | 'creator';
  exp: number;
};

const mockFetch = vi.fn();
const cookieGet = vi.fn(() => ({ value: 'cookie-jwt' }));
const decodeToken = vi.fn<() => MockSession>(() => ({
  username: 'ops-admin',
  role: 'admin' as const,
  exp: Math.floor(Date.now() / 1000) + 3600,
}));

vi.stubGlobal('fetch', mockFetch);
vi.mock('next/headers', () => ({
  cookies: vi.fn(async () => ({ get: cookieGet })),
}));
vi.mock('@/lib/auth', () => ({
  COOKIE_NAME: 'aicity_token',
  decodeToken,
}));

const { proxyAgentTemplates } = await import('@/lib/agent-templates');
const { GET: LIST } = await import('@/app/api/agent-templates/route');
const { POST: REVIEW } = await import(
  '@/app/api/agent-templates/[id]/review/route'
);

function jsonResponse(status: number, body: unknown) {
  return { status, json: async () => body };
}

beforeEach(() => {
  vi.clearAllMocks();
  cookieGet.mockReturnValue({ value: 'cookie-jwt' });
  decodeToken.mockReturnValue({
    username: 'ops-admin',
    role: 'admin',
    exp: Math.floor(Date.now() / 1000) + 3600,
  });
  process.env.ADMIN_PORTAL_A2A_URL = 'http://a2a-gateway:8083';
  process.env.A2A_OPERATOR_API_KEY = 'operator-secret';
});

describe('agent template operator proxy', () => {
  it('requires the operator key', async () => {
    delete process.env.A2A_OPERATOR_API_KEY;

    const response = await proxyAgentTemplates('/v1/operator/npc-templates');

    expect(response.status).toBe(500);
    expect(await response.json()).toMatchObject({ error: { code: 'R_500' } });
    expect(mockFetch).not.toHaveBeenCalled();
  });

  it('forwards submitted list with owner key', async () => {
    mockFetch.mockResolvedValueOnce(jsonResponse(200, { drafts: [] }));

    const response = await LIST(new Request('http://localhost/api/agent-templates?status=submitted'));

    expect(response.status).toBe(200);
    expect(mockFetch).toHaveBeenCalledWith(
      'http://a2a-gateway:8083/v1/operator/npc-templates?status=submitted',
      expect.objectContaining({ method: 'GET' }),
    );
    const headers = mockFetch.mock.calls[0][1].headers as Headers;
    expect(headers.get('X-Operator-Key')).toBe('operator-secret');
  });

  it('rejects non-admin sessions', async () => {
    decodeToken.mockReturnValueOnce({
      username: 'creator',
      role: 'creator',
      exp: Math.floor(Date.now() / 1000) + 3600,
    });

    const response = await REVIEW(
      new Request('http://localhost/api/agent-templates/1/review', {
        method: 'POST',
        body: JSON.stringify({ action: 'approve' }),
      }),
      { params: Promise.resolve({ id: '1' }) },
    );

    expect(response.status).toBe(403);
    expect(mockFetch).not.toHaveBeenCalled();
  });

  it('forwards approval with reviewer identity', async () => {
    mockFetch.mockResolvedValueOnce(jsonResponse(200, { id: 1, status: 'approved' }));

    const response = await REVIEW(
      new Request('http://localhost/api/agent-templates/1/review', {
        method: 'POST',
        body: JSON.stringify({ action: 'approve' }),
      }),
      { params: Promise.resolve({ id: '1' }) },
    );

    expect(response.status).toBe(200);
    expect(mockFetch).toHaveBeenCalledWith(
      'http://a2a-gateway:8083/v1/operator/npc-templates/1/approve',
      expect.objectContaining({ method: 'POST' }),
    );
    const init = mockFetch.mock.calls[0][1];
    expect(JSON.parse(init.body)).toEqual({ reviewer: 'ops-admin' });
  });
});
