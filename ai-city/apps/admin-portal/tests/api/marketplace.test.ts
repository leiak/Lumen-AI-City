import { beforeEach, describe, expect, it, vi } from 'vitest';

const mockFetch = vi.fn();
const cookieGet = vi.fn(() => ({ value: 'cookie-jwt' }));
const decodeToken = vi.fn(() => ({
  username: 'admin',
  role: 'admin',
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

const { proxyMarketplace } = await import('@/lib/marketplace');
const {
  GET: GET_NPC,
  POST: POST_NPC,
} = await import('@/app/api/marketplace/npc-templates/route');
const {
  GET: GET_SAGA,
  POST: POST_SAGA,
} = await import('@/app/api/marketplace/saga-templates/route');
const { POST: POST_PURCHASE } = await import(
  '@/app/api/marketplace/purchase/route'
);
const { GET: GET_INVENTORY } = await import(
  '@/app/api/marketplace/inventory/[user_id]/route'
);
const { GET: GET_REVENUE } = await import(
  '@/app/api/marketplace/revenue/[creator_id]/route'
);

function jsonResponse(status: number, body: unknown) {
  return {
    status,
    json: async () => body,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  cookieGet.mockReturnValue({ value: 'cookie-jwt' });
  decodeToken.mockReturnValue({
    username: 'admin',
    role: 'admin',
    exp: Math.floor(Date.now() / 1000) + 3600,
  });
  process.env.ADMIN_PORTAL_ECONOMY_URL = 'http://economy-service:8005';
});

describe('proxyMarketplace', () => {
  it('requires the economy URL', async () => {
    delete process.env.ADMIN_PORTAL_ECONOMY_URL;

    const response = await proxyMarketplace('/v1/marketplace/npc-templates');

    expect(response.status).toBe(500);
    expect(await response.json()).toMatchObject({
      error: { code: 'R_500' },
    });
    expect(mockFetch).not.toHaveBeenCalled();
  });

  it('forwards method, body, auth, and upstream errors', async () => {
    mockFetch.mockResolvedValueOnce(
      jsonResponse(402, { detail: { code: 'R_022', msg: 'no gold' } }),
    );

    const response = await proxyMarketplace(
      '/v1/marketplace/purchase',
      {
        method: 'POST',
        body: JSON.stringify({ template_id: 42 }),
      },
      'cookie-jwt',
    );

    expect(response.status).toBe(402);
    expect(await response.json()).toEqual({
      detail: { code: 'R_022', msg: 'no gold' },
    });
    expect(mockFetch).toHaveBeenCalledWith(
      'http://economy-service:8005/v1/marketplace/purchase',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({ template_id: 42 }),
        headers: expect.any(Headers),
      }),
    );
    const headers = mockFetch.mock.calls[0][1].headers as Headers;
    expect(headers.get('Authorization')).toBe('Bearer cookie-jwt');
    expect(headers.get('Content-Type')).toBe('application/json');
  });

  it('returns 502 when economy-service is unreachable', async () => {
    mockFetch.mockRejectedValueOnce(new Error('connection refused'));

    const response = await proxyMarketplace('/v1/marketplace/npc-templates');

    expect(response.status).toBe(502);
    expect(await response.json()).toMatchObject({
      error: { code: 'R_502' },
    });
  });
});

describe('marketplace template proxies', () => {
  it('GET npc templates forwards query and auth', async () => {
    mockFetch.mockResolvedValueOnce(jsonResponse(200, []));

    const response = await GET_NPC(
      new Request('http://localhost/api/marketplace/npc-templates?status=live'),
    );

    expect(response.status).toBe(200);
    expect(mockFetch).toHaveBeenCalledWith(
      'http://economy-service:8005/v1/marketplace/npc-templates?status=live',
      expect.objectContaining({ method: 'GET' }),
    );
    const headers = mockFetch.mock.calls[0][1].headers as Headers;
    expect(headers.get('Authorization')).toBe('Bearer cookie-jwt');
  });

  it('POST npc template requires creator or admin', async () => {
    decodeToken.mockReturnValueOnce({
      username: 'player',
      role: 'player',
      exp: Math.floor(Date.now() / 1000) + 3600,
    });

    const response = await POST_NPC(
      new Request('http://localhost/api/marketplace/npc-templates', {
        method: 'POST',
        body: JSON.stringify({ name: 'NPC' }),
      }),
    );

    expect(response.status).toBe(403);
    expect(await response.json()).toMatchObject({
      error: { code: 'R_027' },
    });
    expect(mockFetch).not.toHaveBeenCalled();
  });

  it('POST npc template proxies creator body', async () => {
    decodeToken.mockReturnValueOnce({
      username: 'creator',
      role: 'creator',
      exp: Math.floor(Date.now() / 1000) + 3600,
    });
    mockFetch.mockResolvedValueOnce(jsonResponse(201, { id: 41 }));

    const body = { name: 'NPC', price_gold: 100 };
    const response = await POST_NPC(
      new Request('http://localhost/api/marketplace/npc-templates', {
        method: 'POST',
        body: JSON.stringify(body),
      }),
    );

    expect(response.status).toBe(201);
    expect(mockFetch).toHaveBeenCalledWith(
      'http://economy-service:8005/v1/marketplace/npc-templates',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify(body),
      }),
    );
  });

  it('GET saga templates forwards query and auth', async () => {
    mockFetch.mockResolvedValueOnce(jsonResponse(200, []));

    await GET_SAGA(new Request('http://localhost/api/marketplace/saga-templates'));

    expect(mockFetch).toHaveBeenCalledWith(
      'http://economy-service:8005/v1/marketplace/saga-templates',
      expect.objectContaining({ method: 'GET' }),
    );
  });

  it('POST saga template proxies creator body', async () => {
    decodeToken.mockReturnValueOnce({
      username: 'creator',
      role: 'creator',
      exp: Math.floor(Date.now() / 1000) + 3600,
    });
    mockFetch.mockResolvedValueOnce(jsonResponse(201, { id: 43 }));

    const body = { name: 'Saga', price_gold: 100 };
    const response = await POST_SAGA(
      new Request('http://localhost/api/marketplace/saga-templates', {
        method: 'POST',
        body: JSON.stringify(body),
      }),
    );

    expect(response.status).toBe(201);
    expect(mockFetch).toHaveBeenCalledWith(
      'http://economy-service:8005/v1/marketplace/saga-templates',
      expect.objectContaining({ method: 'POST', body: JSON.stringify(body) }),
    );
  });
});

describe('marketplace action proxies', () => {
  it('POST purchase proxies a signed-in user', async () => {
    decodeToken.mockReturnValueOnce({
      username: 'player',
      role: 'player',
      exp: Math.floor(Date.now() / 1000) + 3600,
    });
    mockFetch.mockResolvedValueOnce(jsonResponse(200, { purchase_id: 71 }));

    const body = {
      template_kind: 'npc',
      template_id: 42,
      idempotency_key: 'key-1',
    };
    const response = await POST_PURCHASE(
      new Request('http://localhost/api/marketplace/purchase', {
        method: 'POST',
        body: JSON.stringify(body),
      }),
    );

    expect(response.status).toBe(200);
    expect(mockFetch).toHaveBeenCalledWith(
      'http://economy-service:8005/v1/marketplace/purchase',
      expect.objectContaining({ method: 'POST', body: JSON.stringify(body) }),
    );
  });

  it('GET inventory forwards user and pagination', async () => {
    mockFetch.mockResolvedValueOnce(jsonResponse(200, []));

    const response = await GET_INVENTORY(
      new Request(
        'http://localhost/api/marketplace/inventory/user-1?limit=20&offset=5',
      ),
      { params: Promise.resolve({ user_id: 'user-1' }) },
    );

    expect(response.status).toBe(200);
    expect(mockFetch).toHaveBeenCalledWith(
      'http://economy-service:8005/v1/marketplace/inventory/user-1?limit=20&offset=5',
      expect.objectContaining({ method: 'GET' }),
    );
  });

  it('GET inventory rejects players', async () => {
    decodeToken.mockReturnValueOnce({
      username: 'player',
      role: 'player',
      exp: Math.floor(Date.now() / 1000) + 3600,
    });

    const response = await GET_INVENTORY(
      new Request('http://localhost/api/marketplace/inventory/user-1'),
      { params: Promise.resolve({ user_id: 'user-1' }) },
    );

    expect(response.status).toBe(403);
    expect(mockFetch).not.toHaveBeenCalled();
  });

  it('GET revenue forwards creator and pagination', async () => {
    mockFetch.mockResolvedValueOnce(jsonResponse(200, []));

    const response = await GET_REVENUE(
      new Request(
        'http://localhost/api/marketplace/revenue/creator-1?limit=10&offset=2',
      ),
      { params: Promise.resolve({ creator_id: 'creator-1' }) },
    );

    expect(response.status).toBe(200);
    expect(mockFetch).toHaveBeenCalledWith(
      'http://economy-service:8005/v1/marketplace/revenue/creator-1?limit=10&offset=2',
      expect.objectContaining({ method: 'GET' }),
    );
  });

  it('GET revenue rejects players', async () => {
    decodeToken.mockReturnValueOnce({
      username: 'player',
      role: 'player',
      exp: Math.floor(Date.now() / 1000) + 3600,
    });

    const response = await GET_REVENUE(
      new Request('http://localhost/api/marketplace/revenue/creator-1'),
      { params: Promise.resolve({ creator_id: 'creator-1' }) },
    );

    expect(response.status).toBe(403);
    expect(mockFetch).not.toHaveBeenCalled();
  });
});
