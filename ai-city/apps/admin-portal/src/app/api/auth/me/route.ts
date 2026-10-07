import { NextResponse } from 'next/server';
import { cookies } from 'next/headers';
import { COOKIE_NAME, decodeToken } from '@/lib/auth';

function getJwtSecret(): string {
  return (
    process.env.ADMIN_PORTAL_JWT_SECRET ||
    process.env.JWT_SECRET ||
    'dev-secret-change-me'
  );
}

export async function GET() {
  const cookieStore = await cookies();
  const token = cookieStore.get(COOKIE_NAME)?.value ?? '';
  const session = decodeToken(token, getJwtSecret());
  if (!session || !session.username) {
    return NextResponse.json(
      { error: { code: 'R_401', msg: 'not authenticated' } },
      { status: 401 }
    );
  }
  return NextResponse.json({
    username: session.username,
    role: session.role ?? 'admin',
  });
}
