import { SagaTemplateDetail } from './SagaTemplateDetail';

export default async function SagaTemplateDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return <SagaTemplateDetail templateId={Number(id)} />;
}
