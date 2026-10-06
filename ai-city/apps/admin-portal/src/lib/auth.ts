/** Phase C.4 — admin-portal auth helpers.
 *
 *  Implements HS256 JWT sign + verify using only Node's built-in ``crypto``
 *  module. No third-party auth library is introduced; this keeps the
 *  dependency surface tight per the C.4 constraint of "NO 3rd-party auth lib".
 *
 *  **Why hand-roll?** The api-gateway already uses HS256 (golang-jwt/jwt/v5)
 *  with ``sub`` / ``uname`` / ``exp`` / ``iat`` claims. We don't share the
 *  secret for admin-portal's cookie session — admin-portal issues its own
 *  token with an extra ``role`` claim so middleware can gate /bt-editor
 *  and /api/bt/* without round-tripping to api-gateway.
 *
 *  Security notes:
 *    - alg is hard-pinned to HS256. Tokens with ``alg=none`` or any other
 *      value are rejected before signature verification (defends against
 *      the classic "alg confusion" attack).
 *    - Signature is timing-safe compared via ``crypto.timingSafeEqual``.
 *    - ``decodeToken`` returns ``null`` for any failure mode (expired,
 *      wrong secret, missing role, malformed). Callers branch on null.
 */

import { createHmac, timingSafeEqual } from 'node:crypto';

export type Role = 'player' | 'admin';

export interface Session {
  username: string;
  role: Role;
  /** Unix epoch seconds — matches JWT ``exp`` claim. */
  exp: number;
}

/** Cookie name for the admin-portal session token. */
export const COOKIE_NAME = 'aicity_token';

/** 7-day session lifetime (seconds). */
export const SESSION_MAX_AGE_SEC = 60 * 60 * 24 * 7;

const HEADER_B64U = Buffer.from(
  JSON.stringify({ alg: 'HS256', typ: 'JWT' }),
).toString('base64url');

function b64urlEncode(buf: Buffer | string): string {
  return Buffer.from(buf).toString('base64url');
}

function b64urlDecodeToString(s: string): string {
  return Buffer.from(s, 'base64url').toString('utf8');
}

/** Sign a session as a HS256 JWT. ``ttlSec`` controls the ``exp`` claim. */
export function signToken(
  session: Omit<Session, 'exp'>,
  secret: string,
  ttlSec: number,
): string {
  if (!secret) throw new Error('auth: secret is required');
  const payload: Session = {
    username: session.username,
    role: session.role,
    exp: Math.floor(Date.now() / 1000) + ttlSec,
  };
  const body = b64urlEncode(JSON.stringify(payload));
  const sig = createHmac('sha256', secret)
    .update(`${HEADER_B64U}.${body}`)
    .digest('base64url');
  return `${HEADER_B64U}.${body}.${sig}`;
}

/**
 * Verify a HS256 JWT and return the decoded session, or null on any failure
 * (bad signature, expired, wrong alg, missing role, malformed).
 */
export function decodeToken(token: string, secret: string): Session | null {
  if (!token || !secret) return null;

  const parts = token.split('.');
  if (parts.length !== 3) return null;
  const [headerB64, bodyB64, sigB64] = parts;

  // 1. alg pinning — refuse alg=none / HS384 / RS256 etc.
  let header: { alg?: string; typ?: string };
  try {
    header = JSON.parse(b64urlDecodeToString(headerB64));
  } catch {
    return null;
  }
  if (header.alg !== 'HS256') return null;

  // 2. signature verification (timing-safe)
  const expected = createHmac('sha256', secret)
    .update(`${headerB64}.${bodyB64}`)
    .digest();
  let actual: Buffer;
  try {
    actual = Buffer.from(sigB64, 'base64url');
  } catch {
    return null;
  }
  if (actual.length !== expected.length) return null;
  if (!timingSafeEqual(actual, expected)) return null;

  // 3. payload parse + role + exp checks
  let payload: { username?: unknown; role?: unknown; exp?: unknown };
  try {
    payload = JSON.parse(b64urlDecodeToString(bodyB64));
  } catch {
    return null;
  }
  if (typeof payload.username !== 'string' || payload.username.length === 0) {
    return null;
  }
  if (payload.role !== 'player' && payload.role !== 'admin') return null;
  if (typeof payload.exp !== 'number') return null;
  // Allow a 30s clock skew for the expiry check.
  if (payload.exp < Math.floor(Date.now() / 1000) - 30) return null;

  return {
    username: payload.username,
    role: payload.role,
    exp: payload.exp,
  };
}

/** Type guard for "is this session an admin?". */
export function isAdmin(session: Session | null): boolean {
  return session?.role === 'admin';
}
