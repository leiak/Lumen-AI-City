/** POST /api/bt/[npc_id]/[tree_name]/simulate — proxy to bt-editor-api simulate endpoint. */

import { NextRequest, NextResponse } from 'next/server';
import { safeName } from '@/lib/safe_name';

const BACKEND = process.env.BT_EDITOR_API_URL ?? 'http://localhost:8090';
/** Maximum request body size — matches C.2 limit. Larger POSTs → 413 R_021. */
const MAX_BODY_BYTES = 256 * 1024;

export async function POST(
  req: NextRequest,
  ctx: { params: Promise<{ npc_id: string; tree_name: string }> },
): Promise<NextResponse> {
  const { npc_id, tree_name } = await ctx.params;
  try {
    safeName(npc_id, 'npc_id');
    safeName(tree_name, 'tree_name');
  } catch (e) {
    return NextResponse.json(
      { detail: { code: 'R_019', msg: String(e) } },
      { status: 422 },
    );
  }

  const cl = req.headers.get('content-length');
  if (cl && Number(cl) > MAX_BODY_BYTES) {
    return NextResponse.json(
      { detail: { code: 'R_021', msg: `body too large: ${cl} > ${MAX_BODY_BYTES}` } },
      { status: 413 },
    );
  }

  const bodyText = await req.text();
  if (bodyText.length > MAX_BODY_BYTES) {
    return NextResponse.json(
      { detail: { code: 'R_021', msg: `body too large: ${bodyText.length} > ${MAX_BODY_BYTES}` } },
      { status: 413 },
    );
  }

  const url = `${BACKEND}/api/v1/bt/${encodeURIComponent(npc_id)}/${encodeURIComponent(tree_name)}/simulate`;
  try {
    const res = await fetch(url, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: bodyText,
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