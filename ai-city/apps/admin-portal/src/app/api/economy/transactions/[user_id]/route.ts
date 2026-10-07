import { proxyGet, requireAdmin } from '@/lib/economy';

export async function GET(req: Request, { params }: { params: Promise<{ user_id: string }> }) {
  const authFail = await requireAdmin();
  if (authFail) return authFail;
  const { user_id } = await params;
  const incoming = new URL(req.url).searchParams;
  const qs = new URLSearchParams({
    limit: incoming.get('limit') ?? '50',
    offset: incoming.get('offset') ?? '0',
  });
  return proxyGet(`/api/v1/transactions/${encodeURIComponent(user_id)}?${qs.toString()}`);
}
