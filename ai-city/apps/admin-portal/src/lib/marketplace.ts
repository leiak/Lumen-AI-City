import { cookies } from 'next/headers';
import { COOKIE_NAME, decodeToken, type Role, type Session } from '@/lib/auth';

const ALL_ROLES: readonly Role[] = ['player', 'creator', 'admin'];

function getJwtSecret(): string {
  return (
    process.env.ADMIN_PORTAL_JWT_SECRET ||
    process.env.JWT_SECRET ||
    'dev-secret-change-me'
  );
}

export async function requireMarketplaceSession(
  allowedRoles: readonly Role[] = ALL_ROLES,
): Promise<Response | { session: Session; token: string }> {
  const token = (await cookies()).get(COOKIE_NAME)?.value;
  const session = token ? decodeToken(token, getJwtSecret()) : null;
  if (!token || !session) {
    return Response.json(
      { error: { code: 'R_401', msg: 'auth required' } },
      { status: 401 },
    );
  }
  if (!allowedRoles.includes(session.role)) {
    return Response.json(
      {
        error: {
          code: allowedRoles.length === 1 && allowedRoles[0] === 'admin' ? 'R_026' : 'R_027',
          msg: 'insufficient role',
        },
      },
      { status: 403 },
    );
  }
  return { session, token };
}

export async function proxyMarketplace(
  path: string,
  init: RequestInit = {},
  token?: string,
): Promise<Response> {
  const base = process.env.ADMIN_PORTAL_ECONOMY_URL;
  if (!base) {
    return Response.json(
      { error: { code: 'R_500', msg: 'ADMIN_PORTAL_ECONOMY_URL not set' } },
      { status: 500 },
    );
  }

  const headers = new Headers(init.headers);
  headers.set('Content-Type', 'application/json');
  if (token) headers.set('Authorization', `Bearer ${token}`);

  try {
    const response = await fetch(`${base}${path}`, {
      ...init,
      method: init.method ?? 'GET',
      headers,
      cache: 'no-store',
    });
    const body = await response.json().catch(() => ({}));
    return Response.json(body, { status: response.status });
  } catch (error) {
    return Response.json(
      {
        error: {
          code: 'R_502',
          msg: `economy-service unreachable: ${(error as Error).message}`,
        },
      },
      { status: 502 },
    );
  }
}
