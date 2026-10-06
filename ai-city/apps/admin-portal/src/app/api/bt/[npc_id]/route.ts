/** GET /api/bt/[npc_id] — proxy to bt-editor-api list endpoint. */

import { NextRequest, NextResponse } from 'next/server';

const BACKEND = process.env.BT_EDITOR_API_URL ?? 'http://localhost:8090';

export async function GET(
  _req: NextRequest,
  ctx: { params: { npc_id: string } },
): Promise<NextResponse> {
  const { npc_id } = ctx.params;
  const url = `${BACKEND}/api/v1/bt/${encodeURIComponent(npc_id)}`;
  try {
    const res = await fetch(url, { cache: 'no-store' });
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