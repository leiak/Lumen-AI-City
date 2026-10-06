/** Typed fetch wrappers for the BT editor API.
 *
 *  Talks to the local Next.js API routes under `/api/bt/...` which in turn
 *  proxy to the upstream `bt-editor-api` FastAPI service.
 */

import type { BTJSON } from './bt_loader';

const BASE = '/api/bt';

export interface BTTreeSummary {
  name: string;
  version: number;
  updated_at: string;
}

export interface BTTreeFull {
  npc_id: string;
  name: string;
  tree_json: BTJSON;
  version: number;
  created_at: string;
  updated_at: string;
}

export interface BTSimulateTraceEntry {
  tick: number;
  node_id: string;
  status: string;
  message?: string;
}

export interface BTSimulateRequest {
  world_state?: Record<string, unknown>;
  /** optional stop condition forwarded to the backend */
  max_ticks?: number;
}

export interface BTSimulateResponse {
  npc_id: string;
  name: string;
  final_status: string;
  trace: BTSimulateTraceEntry[];
}

async function handle<T>(res: Response): Promise<T> {
  if (!res.ok) {
    const text = await res.text().catch(() => '');
    throw new Error(
      `BT API ${res.status} ${res.statusText}: ${text.slice(0, 200)}`,
    );
  }
  return res.json() as Promise<T>;
}

/** GET /api/bt/{npc_id} — list all trees for an NPC. */
export async function listBtTrees(npcId: string): Promise<BTTreeSummary[]> {
  const res = await fetch(`${BASE}/${encodeURIComponent(npcId)}`, {
    method: 'GET',
    headers: { 'Content-Type': 'application/json' },
  });
  return handle<BTTreeSummary[]>(res);
}

/** GET /api/bt/{npc_id}/{tree_name} — fetch one tree. */
export async function getBtTree(
  npcId: string,
  treeName: string,
): Promise<BTTreeFull> {
  const res = await fetch(
    `${BASE}/${encodeURIComponent(npcId)}/${encodeURIComponent(treeName)}`,
    { method: 'GET', headers: { 'Content-Type': 'application/json' } },
  );
  return handle<BTTreeFull>(res);
}

/** POST /api/bt/{npc_id}/{tree_name} — upsert a tree.
 *
 *  Body is the raw BTJSON document; the proxy (and bt-editor-api)
 *  unwrap it into the FastAPI `SaveTreeRequest` shape.
 */
export async function saveBtTree(
  npcId: string,
  treeName: string,
  treeJson: BTJSON,
): Promise<BTTreeFull> {
  const res = await fetch(
    `${BASE}/${encodeURIComponent(npcId)}/${encodeURIComponent(treeName)}`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(treeJson),
    },
  );
  return handle<BTTreeFull>(res);
}

/** POST /api/bt/{npc_id}/{tree_name}/simulate — dry-run tick with trace log. */
export async function simulateBtTree(
  npcId: string,
  treeName: string,
  payload: BTSimulateRequest,
): Promise<BTSimulateResponse> {
  const res = await fetch(
    `${BASE}/${encodeURIComponent(npcId)}/${encodeURIComponent(treeName)}/simulate`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    },
  );
  return handle<BTSimulateResponse>(res);
}
