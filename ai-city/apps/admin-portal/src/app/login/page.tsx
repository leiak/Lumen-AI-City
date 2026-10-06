/** /login — minimal admin-portal login form.
 *
 *  Phase C.4 deliberately keeps this single-screen: username + password
 *  fields, posts to ``/api/auth/login``, on success the API sets the
 *  ``aicity_token`` cookie and redirects back to ``?next=…``.
 *
 *  Server component: no client state needed; the form is a plain HTML
 *  ``<form action="…">`` so it works without JS hydration. The ?next
 *  param is preserved across the round-trip via a hidden input.
 */

import { cookies } from 'next/headers';
import { redirect } from 'next/navigation';
import { COOKIE_NAME, decodeToken, isAdmin } from '@/lib/auth';

export const dynamic = 'force-dynamic';

interface SearchParams {
  next?: string;
  error?: string;
}

const SAFE_NEXT = /^\/[a-zA-Z0-9_\-\/]*$/;

function sanitizeNext(next: string | undefined): string {
  if (!next) return '/';
  // Reject absolute URLs and protocol-relative paths (open-redirect guard).
  if (!next.startsWith('/') || next.startsWith('//')) return '/';
  if (!SAFE_NEXT.test(next)) return '/';
  return next;
}

function getJwtSecret(): string {
  return (
    process.env.ADMIN_PORTAL_JWT_SECRET ||
    process.env.JWT_SECRET ||
    'dev-secret-change-me'
  );
}

export default async function LoginPage({
  searchParams,
}: {
  searchParams: Promise<SearchParams>;
}) {
  const sp = await searchParams;
  const next = sanitizeNext(sp?.next);

  // Already logged in? Skip the form and bounce straight to ?next.
  const cookieStore = await cookies();
  const existing = cookieStore.get(COOKIE_NAME)?.value;
  if (existing && isAdmin(decodeToken(existing, getJwtSecret()))) {
    redirect(next as unknown as __next_route_internal_types__.RouteImpl<string>);
  }

  return (
    <main className="min-h-screen flex items-center justify-center bg-gray-50">
      <div className="w-full max-w-sm p-8 bg-white rounded-lg border border-gray-200 shadow-sm">
        <h1 className="text-2xl font-bold mb-1">Admin Login</h1>
        <p className="text-sm text-gray-500 mb-6">
          AI City 运营后台 · Phase C.4
        </p>
        <form action="/api/auth/login" method="POST" className="space-y-4">
          <input type="hidden" name="next" value={next} />
          <div>
            <label
              htmlFor="username"
              className="block text-sm font-medium text-gray-700 mb-1"
            >
              Username
            </label>
            <input
              id="username"
              name="username"
              type="text"
              autoComplete="username"
              required
              className="w-full px-3 py-2 border border-gray-300 rounded focus:outline-none focus:ring-2 focus:ring-blue-500"
            />
          </div>
          <div>
            <label
              htmlFor="password"
              className="block text-sm font-medium text-gray-700 mb-1"
            >
              Password
            </label>
            <input
              id="password"
              name="password"
              type="password"
              autoComplete="current-password"
              required
              className="w-full px-3 py-2 border border-gray-300 rounded focus:outline-none focus:ring-2 focus:ring-blue-500"
            />
          </div>
          {sp?.error && (
            <p
              data-testid="login-error"
              className="text-sm text-red-600 bg-red-50 px-3 py-2 rounded"
            >
              {sp.error === 'invalid'
                ? '用户名或密码错误'
                : '登录失败，请重试'}
            </p>
          )}
          <button
            type="submit"
            className="w-full py-2 px-4 bg-blue-600 text-white rounded font-medium hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500"
          >
            登录
          </button>
        </form>
        <p className="mt-6 text-xs text-gray-400">
          默认账号 admin / adminpass（pgcrypto seed）。生产环境请通过
          <code className="px-1">ADMIN_USERNAME</code> /
          <code className="px-1">ADMIN_PASSWORD</code> 环境变量覆盖。
        </p>
      </div>
    </main>
  );
}
