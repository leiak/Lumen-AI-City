import { proxyAgentTemplates, requireAgentTemplateAdmin } from '@/lib/agent-templates';

export async function POST(req: Request, ctx: { params: Promise<{ id: string }> }) {
  const auth = await requireAgentTemplateAdmin();
  if (auth instanceof Response) return auth;
  const { id } = await ctx.params;
  const body = await req.json().catch(() => ({}));
  const action = body?.action === 'reject' ? 'reject' : 'approve';
  return proxyAgentTemplates(
    `/v1/operator/npc-templates/${encodeURIComponent(id)}/${action}`,
    { method: 'POST', body: JSON.stringify({ reviewer: auth.session.username }) },
  );
}
