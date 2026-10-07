/** GET /api/players — list the seeded demo + admin players.
 *
 *  Used by the admin-portal Layout's PlayerSelector to populate the global
 *  player dropdown (consumed by /wallet, /transactions, /admin pages).
 *  Returns the demo + admin rows from the shared `player` table.
 *
 *  Auth: JWT admin cookie (aicity_token) — see src/lib/auth.ts. Non-admin
 *  callers get 401.
 *
 *  Responses:
 *    200 — { players: [{ id, username, role }, ...] }
 *    401 — admin auth required
 *    500 — DATABASE_URL not set / DB query failed
 */

import { NextResponse } from 'next/server';
import { Client } from 'pg';
import { cookies } from 'next/headers';
import { COOKIE_NAME, decodeToken, isAdmin } from '@/lib/auth';

export const dynamic = 'force-dynamic';

function getJwtSecret(): string {
  return (
    process.env.ADMIN_PORTAL_JWT_SECRET ||
    process.env.JWT_SECRET ||
    'dev-secret-change-me'
  );
}

export async function GET(_req: Request) {
  // Auth check
  const cookieStore = await cookies();
  const token = cookieStore.get(COOKIE_NAME)?.value ?? '';
  const session = decodeToken(token, getJwtSecret());
  if (!isAdmin(session)) {
    return NextResponse.json(
      { error: { code: 'R_401', msg: 'admin auth required' } },
      { status: 401 }
    );
  }

  const dbUrl = process.env.DATABASE_URL;
  if (!dbUrl) {
    return NextResponse.json(
      { error: { code: 'R_500', msg: 'DATABASE_URL not set' } },
      { status: 500 }
    );
  }

  const client = new Client({ connectionString: dbUrl });
  try {
    await client.connect();
    const result = await client.query(
      `SELECT id, username, role FROM player
       WHERE username IN ('demo', 'admin')
       ORDER BY username`
    );
    return NextResponse.json({ players: result.rows });
  } catch (err) {
    return NextResponse.json(
      { error: { code: 'R_500', msg: `DB query failed: ${(err as Error).message}` } },
      { status: 500 }
    );
  } finally {
    await client.end();
  }
}