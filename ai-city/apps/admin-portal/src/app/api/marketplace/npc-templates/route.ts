import { proxyMarketplace, requireMarketplaceSession } from '@/lib/marketplace';

export async function GET(req: Request) {
  const auth = await requireMarketplaceSession();
  if (auth instanceof Response) return auth;
  const query = new URL(req.url).search;
  return proxyMarketplace(
    `/v1/marketplace/npc-templates${query}`,
    {},
    auth.token,
  );
}

export async function POST(req: Request) {
  const auth = await requireMarketplaceSession(['creator', 'admin']);
  if (auth instanceof Response) return auth;
  const body = await req.json().catch(() => ({}));
  return proxyMarketplace(
    '/v1/marketplace/npc-templates',
    { method: 'POST', body: JSON.stringify(body) },
    auth.token,
  );
}
