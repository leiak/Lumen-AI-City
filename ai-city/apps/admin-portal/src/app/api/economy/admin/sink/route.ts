import { proxyPost, requireAdmin } from '@/lib/economy';

export async function POST(req: Request) {
  const authFail = await requireAdmin();
  if (authFail) return authFail;
  const body = await req.json().catch(() => ({}));
  return proxyPost('/api/v1/admin/central-bank/sink', body, true);
}
