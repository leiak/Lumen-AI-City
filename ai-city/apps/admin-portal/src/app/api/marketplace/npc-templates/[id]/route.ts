import { proxyMarketplace, requireMarketplaceSession } from '@/lib/marketplace';

export async function GET(
  _req: Request,
  { params }: { params: Promise<{ id: string }> },
) {
  const auth = await requireMarketplaceSession();
  if (auth instanceof Response) return auth;
  const { id } = await params;
  return proxyMarketplace(
    `/v1/marketplace/npc-templates/${encodeURIComponent(id)}`,
    {},
    auth.token,
  );
}
