/** GET + POST /api/bt/[npc_id]/[tree_name] — proxy to bt-editor-api. */

import { NextRequest, NextResponse } from 'next/server';

const BACKEND = process.env.BT_EDITOR_API_URL ?? 'http://localhost:8090';

async function forward(
  req: NextRequest,
  method: 'GET' | 'POST',
  npc_id: string,
  tree_name: string,
): Promise<NextResponse> {
  const url = `${BACKEND}/api/v1/bt/${encodeURIComponent(npc_id)}/${encodeURIComponent(tree_name)}`;
  try {
    const init: RequestInit = { method, cache: 'no-store' };
    if (method === 'POST') {
      init.headers = { 'Content-Type': 'application/json' };
      init.body = await req.text();
    }
    const res = await fetch(url, init);
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

export async function GET(
  req: NextRequest,
  ctx: { params: Promise<{ npc_id: string; tree_name: string }> },
): Promise<NextResponse> {
  const { npc_id, tree_name } = await ctx.params;
  return forward(req, 'GET', npc_id, tree_name);
}

export async function POST(
  req: NextRequest,
  ctx: { params: Promise<{ npc_id: string; tree_name: string }> },
): Promise<NextResponse> {
  const { npc_id, tree_name } = await ctx.params;
  return forward(req, 'POST', npc_id, tree_name);
}