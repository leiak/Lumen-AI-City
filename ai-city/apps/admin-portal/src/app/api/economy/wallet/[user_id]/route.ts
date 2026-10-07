import { proxyGet, requireAdmin } from '@/lib/economy';

export async function GET(req: Request, { params }: { params: Promise<{ user_id: string }> }) {
  const authFail = await requireAdmin();
  if (authFail) return authFail;
  const { user_id } = await params;
  return proxyGet(`/api/v1/wallet/${encodeURIComponent(user_id)}`);
}