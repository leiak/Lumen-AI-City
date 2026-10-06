/** Phase C.4 — admin-portal auth helpers (JWT HS256 + role check).
 *
 *  Tests cover:
 *   - signToken + decodeToken roundtrip for admin role
 *   - signToken + decodeToken roundtrip for player role
 *   - decodeToken returns null for expired token
 *   - decodeToken returns null for wrong-secret signature
 *   - decodeToken returns null for malformed/garbage input
 *   - decodeToken returns null for alg=none (security guard)
 *   - decodeToken returns null when role claim is missing or unknown
 *   - isAdmin true/false for both roles
 *   - COOKIE_NAME + SESSION_MAX_AGE constants are exported
 */

import { describe, it, expect } from 'vitest';
import {
  COOKIE_NAME,
  SESSION_MAX_AGE_SEC,
  decodeToken,
  isAdmin,
  signToken,
  type Session,
} from './auth';

const SECRET = 'unit-test-secret-do-not-use-in-prod';

describe('auth constants', () => {
  it('exports a non-empty cookie name', () => {
    expect(COOKIE_NAME.length).toBeGreaterThan(0);
    expect(COOKIE_NAME).toBe('aicity_token');
  });

  it('session max age is 7 days (seconds)', () => {
    expect(SESSION_MAX_AGE_SEC).toBe(60 * 60 * 24 * 7);
  });
});

describe('signToken + decodeToken roundtrip', () => {
  it('roundtrips an admin session', () => {
    const token = signToken(
      { username: 'admin', role: 'admin' },
      SECRET,
      3600,
    );
    const decoded = decodeToken(token, SECRET);
    expect(decoded).not.toBeNull();
    expect(decoded?.username).toBe('admin');
    expect(decoded?.role).toBe('admin');
    expect(decoded?.exp).toBeGreaterThan(Math.floor(Date.now() / 1000));
  });

  it('roundtrips a player session', () => {
    const token = signToken(
      { username: 'demo', role: 'player' },
      SECRET,
      3600,
    );
    const decoded = decodeToken(token, SECRET);
    expect(decoded?.username).toBe('demo');
    expect(decoded?.role).toBe('player');
  });
});

describe('decodeToken failure modes', () => {
  it('returns null for empty string', () => {
    expect(decodeToken('', SECRET)).toBeNull();
  });

  it('returns null for garbage input', () => {
    expect(decodeToken('not.a.jwt', SECRET)).toBeNull();
    expect(decodeToken('just-some-string', SECRET)).toBeNull();
  });

  it('returns null for wrong-secret signature', () => {
    const token = signToken(
      { username: 'admin', role: 'admin' },
      SECRET,
      3600,
    );
    expect(decodeToken(token, 'other-secret')).toBeNull();
  });

  it('returns null for expired token', () => {
    // 60s in the past — well past the 30s clock-skew tolerance.
    const token = signToken(
      { username: 'admin', role: 'admin' },
      SECRET,
      -60,
    );
    expect(decodeToken(token, SECRET)).toBeNull();
  });

  it('returns null for alg=none spoofed token', () => {
    // Hand-craft an alg=none token with admin role — must be rejected.
    const header = Buffer.from(JSON.stringify({ alg: 'none', typ: 'JWT' }))
      .toString('base64url');
    const payload = Buffer.from(
      JSON.stringify({
        username: 'attacker',
        role: 'admin',
        exp: Math.floor(Date.now() / 1000) + 3600,
      }),
    ).toString('base64url');
    const token = `${header}.${payload}.`;
    expect(decodeToken(token, SECRET)).toBeNull();
  });

  it('returns null when role claim is missing', () => {
    // Forge a token without a "role" field — older api-gateway tokens
    // lack role; admin-portal must refuse them.
    const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' }))
      .toString('base64url');
    const payload = Buffer.from(
      JSON.stringify({
        username: 'demo',
        exp: Math.floor(Date.now() / 1000) + 3600,
      }),
    ).toString('base64url');
    const sig = require('node:crypto')
      .createHmac('sha256', SECRET)
      .update(`${header}.${payload}`)
      .digest('base64url');
    const token = `${header}.${payload}.${sig}`;
    expect(decodeToken(token, SECRET)).toBeNull();
  });

  it('returns null when role is neither player nor admin', () => {
    const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' }))
      .toString('base64url');
    const payload = Buffer.from(
      JSON.stringify({
        username: 'someone',
        role: 'superuser',
        exp: Math.floor(Date.now() / 1000) + 3600,
      }),
    ).toString('base64url');
    const sig = require('node:crypto')
      .createHmac('sha256', SECRET)
      .update(`${header}.${payload}`)
      .digest('base64url');
    const token = `${header}.${payload}.${sig}`;
    expect(decodeToken(token, SECRET)).toBeNull();
  });
});

describe('isAdmin', () => {
  it('returns true for admin session', () => {
    const session: Session = {
      username: 'admin',
      role: 'admin',
      exp: Math.floor(Date.now() / 1000) + 3600,
    };
    expect(isAdmin(session)).toBe(true);
  });

  it('returns false for player session', () => {
    const session: Session = {
      username: 'demo',
      role: 'player',
      exp: Math.floor(Date.now() / 1000) + 3600,
    };
    expect(isAdmin(session)).toBe(false);
  });

  it('returns false for null', () => {
    expect(isAdmin(null)).toBe(false);
  });
});
