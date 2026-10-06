/** Phase C.4 — admin-portal route protection middleware.
 *
 *  Gates ``/bt-editor/*`` (UI) and ``/api/bt/*`` (API proxy) on the
 *  presence of a valid admin session cookie. Non-admin / missing cookies
 *  are redirected (UI) or 401 (API). The cookie value is *not* re-verified
 *  here — verification happens in the per-route handler / server component
 *  so we can return a more useful response (e.g. 401 + R_401 JSON envelope).
 *
 *  Why gate on cookie *presence* here but verify in-handler? Because the
 *  middleware runs in the Edge runtime which lacks Node's ``crypto`` for
 *  HMAC verification in some Next.js deployments. By checking only
 *  presence here, we still block anonymous traffic at the edge (cheap,
 *  fast) and let the Node-runtime server component / API route do the
 *  full signature + role check.
 */

import { NextRequest, NextResponse } from 'next/server';

export const COOKIE_NAME = 'aicity_token';

const PROTECTED_PREFIXES = ['/bt-editor', '/api/bt'];

export function middleware(req: NextRequest): NextResponse {
  const { pathname } = req.nextUrl;
  const isProtected = PROTECTED_PREFIXES.some((p) => pathname.startsWith(p));
  if (!isProtected) return NextResponse.next();

  const token = req.cookies.get(COOKIE_NAME)?.value;
  if (!token) {
    // API routes → 401 JSON envelope (matches the rest of admin-portal's
    // R_401 convention used by the bt proxy).
    if (pathname.startsWith('/api/')) {
      return NextResponse.json(
        { detail: { code: 'R_401', msg: 'authentication required' } },
        { status: 401 },
      );
    }
    // UI routes → redirect to /login, preserving the original target.
    const loginUrl = new URL('/login', req.url);
    loginUrl.searchParams.set('next', pathname);
    return NextResponse.redirect(loginUrl);
  }

  // Cookie present — let the request through. The downstream server
  // component / API route will re-decode the JWT and verify role.
  return NextResponse.next();
}

export const config = {
  matcher: ['/bt-editor/:path*', '/api/bt/:path*'],
};
