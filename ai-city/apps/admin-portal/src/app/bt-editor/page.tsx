/** /bt-editor — Server component shell that hands the npc_id query param
 *  to the client editor. Defaults to the Wang boss NPC if no npc_id is given.
 */

import BtEditorClient from './BtEditorClient';

interface SearchParams {
  npc_id?: string;
  tree_name?: string;
}

export default function BtEditorPage({
  searchParams,
}: {
  searchParams?: SearchParams;
}) {
  const npcId = searchParams?.npc_id ?? 'npc_wang_boss_001';
  const initialTreeName = searchParams?.tree_name ?? null;
  return <BtEditorClient initialNpcId={npcId} initialTreeName={initialTreeName} />;
}