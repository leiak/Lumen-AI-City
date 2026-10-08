import { NpcTemplateDetail } from './NpcTemplateDetail';

export default async function NpcTemplateDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return <NpcTemplateDetail templateId={Number(id)} />;
}
