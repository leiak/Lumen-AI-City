import { proxyAgentTemplates, requireAgentTemplateAdmin } from '@/lib/agent-templates';

export async function GET(req: Request) {
  const auth = await requireAgentTemplateAdmin();
  if (auth instanceof Response) return auth;
  const status = new URL(req.url).searchParams.get('status') || 'submitted';
  return proxyAgentTemplates(`/v1/operator/npc-templates?status=${encodeURIComponent(status)}`);
}
