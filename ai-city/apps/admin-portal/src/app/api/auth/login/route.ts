/** POST /api/auth/login — admin-portal credential check + JWT issuance.
 *
 *  Phase C.4 v0 design: hard-coded admin/adminpass check (env-tunable).
 *  We deliberately do **not** round-trip to api-gateway for auth — that
 *  service issues JWTs without a ``role`` claim, which admin-portal's
 *  middleware needs. Signing our own cookie-session JWT keeps the
 *  ``role`` claim local to admin-portal without touching api-gateway.
 *
 *  Request body (JSON or form-urlencoded):
 *    { username: string, password: string, next?: string }
 *
 *  Responses:
 *    200 — { ok: true } + Set-Cookie aicity_token=<jwt>
 *    302 — redirect to ?next=… for form submissions (browser navigation)
 *    401 — { detail: { code: 'R_401', msg: 'invalid credentials' } }
 *
 *  For non-form (fetch) callers we return JSON; for browser form
 *  submissions we return 302. The discriminator is ``Accept`` header.
 */

import { NextRequest, NextResponse } from 'next/server';
import {
  COOKIE_NAME,
  SESSION_MAX_AGE_SEC,
  signToken,
} from '@/lib/auth';

export const dynamic = 'force-dynamic';

const SAFE_NEXT = /^\/[a-zA-Z0-9_\-\/]*$/;

function getAdminCredentials(): { username: string; password: string } {
  return {
    username: process.env.ADMIN_USERNAME || 'admin',
    password: process.env.ADMIN_PASSWORD || 'adminpass',
  };
}

function getJwtSecret(): string {
  return (
    process.env.ADMIN_PORTAL_JWT_SECRET ||
    process.env.JWT_SECRET ||
    'dev-secret-change-me'
  );
}

function sanitizeNext(next: string | null | undefined): string {
  if (!next) return '/';
  if (!next.startsWith('/') || next.startsWith('//')) return '/';
  if (!SAFE_NEXT.test(next)) return '/';
  return next;
}

function wantsJson(req: NextRequest): boolean {
  const accept = req.headers.get('accept') ?? '';
  return accept.includes('application/json');
}

async function readBody(
  req: NextRequest,
): Promise<{ username: string; password: string; next: string }> {
  const ct = req.headers.get('content-type') ?? '';
  if (ct.includes('application/json')) {
    const j = (await req.json().catch(() => ({}))) as Record<string, unknown>;
    return {
      username: String(j.username ?? ''),
      password: String(j.password ?? ''),
      next: sanitizeNext(typeof j.next === 'string' ? j.next : undefined),
    };
  }
  // form-urlencoded (default for HTML <form>)
  const form = await req.formData();
  return {
    username: String(form.get('username') ?? ''),
    password: String(form.get('password') ?? ''),
    next: sanitizeNext(
      typeof form.get('next') === 'string' ? (form.get('next') as string) : undefined,
    ),
  };
}

function isSecure(): boolean {
  return process.env.NODE_ENV === 'production';
}

export async function POST(req: NextRequest): Promise<NextResponse> {
  const { username, password, next } = await readBody(req);

  const creds = getAdminCredentials();
  if (!username || !password || username !== creds.username || password !== creds.password) {
    if (wantsJson(req)) {
      return NextResponse.json(
        { detail: { code: 'R_401', msg: 'invalid credentials' } },
        { status: 401 },
      );
    }
    // Browser form → bounce back to /login with error
    const url = new URL('/login', req.url);
    url.searchParams.set('error', 'invalid');
    if (next !== '/') url.searchParams.set('next', next);
    return NextResponse.redirect(url, { status: 303 });
  }

  const token = signToken(
    { username: creds.username, role: 'admin' },
    getJwtSecret(),
    SESSION_MAX_AGE_SEC,
  );

  const cookieOpts = {
    httpOnly: true,
    secure: isSecure(),
    sameSite: 'lax' as const,
    path: '/',
    maxAge: SESSION_MAX_AGE_SEC,
  };

  if (wantsJson(req)) {
    const res = NextResponse.json({ ok: true, role: 'admin' });
    res.cookies.set(COOKIE_NAME, token, cookieOpts);
    return res;
  }

  // Browser form → 303 redirect to ?next=…
  const res = NextResponse.redirect(new URL(next, req.url), { status: 303 });
  res.cookies.set(COOKIE_NAME, token, cookieOpts);
  return res;
}
