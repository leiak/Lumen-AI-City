import { proxyMarketplace, requireMarketplaceSession } from '@/lib/marketplace';

export async function POST(
  _req: Request,
  { params }: { params: Promise<{ id: string }> },
) {
  const auth = await requireMarketplaceSession(['admin']);
  if (auth instanceof Response) return auth;
  const { id } = await params;
  return proxyMarketplace(
    `/v1/marketplace/saga-templates/${encodeURIComponent(id)}/take-down`,
    { method: 'POST' },
    auth.token,
  );
}
