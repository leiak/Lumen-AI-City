import { proxyMarketplace, requireMarketplaceSession } from '@/lib/marketplace';

export async function GET(
  req: Request,
  { params }: { params: Promise<{ creator_id: string }> },
) {
  const auth = await requireMarketplaceSession(['creator', 'admin']);
  if (auth instanceof Response) return auth;
  const { creator_id } = await params;
  const incoming = new URL(req.url).searchParams;
  const query = new URLSearchParams({
    limit: incoming.get('limit') ?? '50',
    offset: incoming.get('offset') ?? '0',
  });
  return proxyMarketplace(
    `/v1/marketplace/revenue/${encodeURIComponent(creator_id)}?${query}`,
    {},
    auth.token,
  );
}
