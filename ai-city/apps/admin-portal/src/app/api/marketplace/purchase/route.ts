import { proxyMarketplace, requireMarketplaceSession } from '@/lib/marketplace';

export async function POST(req: Request) {
  const auth = await requireMarketplaceSession();
  if (auth instanceof Response) return auth;
  const body = await req.json().catch(() => ({}));
  return proxyMarketplace(
    '/v1/marketplace/purchase',
    { method: 'POST', body: JSON.stringify(body) },
    auth.token,
  );
}
