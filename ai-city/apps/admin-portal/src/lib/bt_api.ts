/** Typed fetch wrappers for the BT editor API.
 *
 *  Talks to the local Next.js API routes under `/api/bt/...` which in turn
 *  proxy to the upstream `bt-editor-api` FastAPI service.
 *
 *  Wire shapes match the C.2 GA contract (see apps/bt-editor-api/src/
 *  bt_editor_api/schemas.py):
 *    - POST /api/v1/bt/{npc_id}/{tree_name}        → SaveTreeRequest{tree_json}
 *    - POST /api/v1/bt/{npc_id}/{tree_name}/simulate → SimulateRequest{tree_json, state, tick_limit}
 *    - SimulateResponse{status, trace[node_id/status/tick_count], final_state}
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

/** Single trace entry returned by the simulate endpoint (C.2 contract). */
export interface BTSimulateTraceEntry {
  node_id: string;
  status: string;
  tick_count: number;
}

/** Optional inner state payload for /simulate — defaults match backend factory. */
export interface BTSimulateState {
  player_position?: [number, number];
  npc_state?: Record<string, unknown>;
  time_of_day?: string;
  max_ticks?: number;
}

/** Request body for /simulate (C.2 contract). */
export interface BTSimulateRequest {
  tree_json: BTJSON;
  state?: BTSimulateState;
  tick_limit?: number;
}

/** Response body for /simulate (C.2 contract). */
export interface BTSimulateResponse {
  status: string;        // "success" | "failure" | "running" | "error"
  trace: BTSimulateTraceEntry[];
  final_state: Record<string, unknown>;
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
 *  Body is wrapped as `{ tree_json: ... }` per C.2's SaveTreeRequest schema.
 *  The proxy forwards it to bt-editor-api verbatim.
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
      body: JSON.stringify({ tree_json: treeJson }),
    },
  );
  return handle<BTTreeFull>(res);
}

/** POST /api/bt/{npc_id}/{tree_name}/simulate — dry-run tick with trace log.
 *
 *  Request/response shapes match C.2 SimulateRequest / SimulateResponse.
 */
export async function simulateBtTree(
  npcId: string,
  treeName: string,
  treeJson: BTJSON,
  tickLimit = 100,
): Promise<BTSimulateResponse> {
  const res = await fetch(
    `${BASE}/${encodeURIComponent(npcId)}/${encodeURIComponent(treeName)}/simulate`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        tree_json: treeJson,
        state: {
          player_position: [0, 0],
          npc_state: {},
          time_of_day: 'noon',
          max_ticks: 100,
        },
        tick_limit: tickLimit,
      }),
    },
  );
  if (!res.ok) {
    const detail = await res.json().catch(() => ({}));
    throw new Error(`simulate failed: ${res.status} ${JSON.stringify(detail)}`);
  }
  return res.json() as Promise<BTSimulateResponse>;
}