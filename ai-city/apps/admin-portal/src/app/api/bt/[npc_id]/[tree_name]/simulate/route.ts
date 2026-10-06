/** POST /api/bt/[npc_id]/[tree_name]/simulate — proxy to bt-editor-api simulate endpoint. */

import { NextRequest, NextResponse } from 'next/server';

const BACKEND = process.env.BT_EDITOR_API_URL ?? 'http://localhost:8090';

export async function POST(
  req: NextRequest,
  ctx: { params: Promise<{ npc_id: string; tree_name: string }> },
): Promise<NextResponse> {
  const { npc_id, tree_name } = await ctx.params;
  const url = `${BACKEND}/api/v1/bt/${encodeURIComponent(npc_id)}/${encodeURIComponent(tree_name)}/simulate`;
  try {
    const res = await fetch(url, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: await req.text(),
      cache: 'no-store',
    });
    const body = await res.text();
    return new NextResponse(body, {
      status: res.status,
      headers: { 'Content-Type': res.headers.get('Content-Type') ?? 'application/json' },
    });
  } catch (err) {
    return NextResponse.json(
      { detail: { code: 'R_PROXY', msg: String(err) } },
      { status: 502 },
    );
  }
}