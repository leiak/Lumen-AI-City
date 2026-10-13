import { cookies } from 'next/headers';
import { COOKIE_NAME, decodeToken, type Session } from '@/lib/auth';

export type AgentTemplateReview = 'approve' | 'reject';

function getJwtSecret(): string {
  return process.env.ADMIN_PORTAL_JWT_SECRET || process.env.JWT_SECRET || 'dev-secret-change-me';
}

export async function requireAgentTemplateAdmin(): Promise<Response | { session: Session }> {
  const token = (await cookies()).get(COOKIE_NAME)?.value;
  const session = token ? decodeToken(token, getJwtSecret()) : null;
  if (!token || !session) {
    return Response.json(
      { error: { code: 'R_401', msg: 'auth required' } },
      { status: 401 },
    );
  }
  if (session.role !== 'admin') {
    return Response.json(
      { error: { code: 'R_026', msg: 'insufficient role' } },
      { status: 403 },
    );
  }
  return { session };
}

export async function proxyAgentTemplates(
  path: string,
  init: RequestInit = {},
): Promise<Response> {
  const base = process.env.ADMIN_PORTAL_A2A_URL || 'http://a2a-gateway:8083';
  const operatorKey = process.env.A2A_OPERATOR_API_KEY;
  if (!operatorKey) {
    return Response.json(
      { error: { code: 'R_500', msg: 'A2A_OPERATOR_API_KEY not set' } },
      { status: 500 },
    );
  }

  const headers = new Headers(init.headers);
  headers.set('Content-Type', 'application/json');
  headers.set('X-Operator-Key', operatorKey);

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
      { error: { code: 'R_502', msg: `a2a-gateway unreachable: ${(error as Error).message}` } },
      { status: 502 },
    );
  }
}
