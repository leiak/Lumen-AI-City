import { proxyGet, requireAdmin } from '@/lib/economy';

export async function GET(req: Request, { params }: { params: Promise<{ user_id: string }> }) {
  const authFail = await requireAdmin();
  if (authFail) return authFail;
  const { user_id } = await params;
  const url = new URL(req.url);
  const limit = url.searchParams.get('limit') ?? '50';
  const offset = url.searchParams.get('offset') ?? '0';
  return proxyGet(`/api/v1/transactions/${encodeURIComponent(user_id)}?limit=${limit}&offset=${offset}`);
}
