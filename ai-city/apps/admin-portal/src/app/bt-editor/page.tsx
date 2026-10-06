/** /bt-editor — Server component shell that hands the npc_id query param
 *  to the client editor. Defaults to the Wang boss NPC if no npc_id is given.
 *
 *  Phase C.4: middleware already blocked anonymous traffic; here we
 *  re-decode the cookie and gate on ``role === 'admin'``. Non-admin
 *  sessions see a friendly 403 page instead of the editor UI.
 */

import { cookies } from 'next/headers';
import BtEditorClient from './BtEditorClient';
import { COOKIE_NAME, decodeToken, isAdmin } from '@/lib/auth';

interface SearchParams {
  npc_id?: string;
  tree_name?: string;
}

function getJwtSecret(): string {
  return (
    process.env.ADMIN_PORTAL_JWT_SECRET ||
    process.env.JWT_SECRET ||
    'dev-secret-change-me'
  );
}

export default async function BtEditorPage({
  searchParams,
}: {
  searchParams: Promise<SearchParams>;
}) {
  const sp = await searchParams;
  const npcId = sp?.npc_id ?? 'npc_wang_boss_001';
  const initialTreeName = sp?.tree_name ?? null;

  const cookieStore = await cookies();
  const token = cookieStore.get(COOKIE_NAME)?.value ?? '';
  const session = decodeToken(token, getJwtSecret());
  if (!isAdmin(session)) {
    return (
      <main className="min-h-screen flex items-center justify-center bg-gray-50">
        <div className="max-w-md p-8 bg-white rounded-lg border border-gray-200 shadow-sm text-center">
          <h1 className="text-2xl font-bold text-red-600 mb-2">403 Forbidden</h1>
          <p className="text-gray-700">
            BT 编辑器仅管理员可访问。当前账号：
            <code className="px-1 mx-1 bg-gray-100 rounded">
              {session?.username ?? 'anonymous'}
            </code>
            （role={session?.role ?? 'none'}）。
          </p>
          <p className="mt-4 text-sm text-gray-500">
            如需访问，请联系超级管理员授予 admin 角色。
          </p>
        </div>
      </main>
    );
  }

  return <BtEditorClient initialNpcId={npcId} initialTreeName={initialTreeName} />;
}