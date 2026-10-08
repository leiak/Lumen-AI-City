import { proxyMarketplace, requireMarketplaceSession } from '@/lib/marketplace';

export async function GET(
  req: Request,
  { params }: { params: Promise<{ user_id: string }> },
) {
  const auth = await requireMarketplaceSession(['admin']);
  if (auth instanceof Response) return auth;
  const { user_id } = await params;
  const incoming = new URL(req.url).searchParams;
  const query = new URLSearchParams({
    limit: incoming.get('limit') ?? '50',
    offset: incoming.get('offset') ?? '0',
  });
  return proxyMarketplace(
    `/v1/marketplace/inventory/${encodeURIComponent(user_id)}?${query}`,
    {},
    auth.token,
  );
}
