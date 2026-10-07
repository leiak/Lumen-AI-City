/** Shared helpers for proxying requests to economy-service. */
import { cookies } from 'next/headers';
import { COOKIE_NAME, decodeToken, isAdmin } from '@/lib/auth';

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
        { status: 503 }
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
