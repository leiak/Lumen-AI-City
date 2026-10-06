import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import {
  listBtTrees,
  getBtTree,
  saveBtTree,
  simulateBtTree,
} from './bt_api';

const NPC = 'npc_wang_boss_001';
const TREE = 'tavern_greeting';

type FetchArgs = [input: string | URL | Request, init?: RequestInit];

function mockFetchOnce(response: Response) {
  const fn = vi.fn<typeof fetch>(
    async () => response,
  );
  vi.stubGlobal('fetch', fn);
  return fn;
}

describe('bt_api wrappers', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('listBtTrees → GET /api/bt/{npc} and returns array', async () => {
    const mockPayload = [
      { name: TREE, version: 1, updated_at: '2026-10-06T00:00:00Z' },
    ];
    const fetchMock = mockFetchOnce(
      new Response(JSON.stringify(mockPayload), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    );

    const trees = await listBtTrees(NPC);

    expect(trees).toEqual(mockPayload);
    expect(fetchMock).toHaveBeenCalledOnce();
    const [url, init] = fetchMock.mock.calls[0] as unknown as FetchArgs;
    expect(url).toBe(`/api/bt/${NPC}`);
    expect((init?.method ?? 'GET')).toBe('GET');
  });

  it('getBtTree → GET /api/bt/{npc}/{name} and parses JSON', async () => {
    const mockPayload = {
      npc_id: NPC,
      name: TREE,
      tree_json: { version: '1.0.0', root: { id: 'r', type: 'sequence' } },
      version: 1,
      created_at: '2026-10-06T00:00:00Z',
      updated_at: '2026-10-06T00:00:00Z',
    };
    const fetchMock = mockFetchOnce(
      new Response(JSON.stringify(mockPayload), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    );

    const tree = await getBtTree(NPC, TREE);

    expect(tree).toEqual(mockPayload);
    const [url] = fetchMock.mock.calls[0] as unknown as FetchArgs;
    expect(url).toBe(`/api/bt/${NPC}/${TREE}`);
  });

  it('getBtTree → throws on 404', async () => {
    mockFetchOnce(new Response('not found', { status: 404 }));

    await expect(getBtTree(NPC, 'missing_tree')).rejects.toThrow(/404/);
  });

  it('saveBtTree → POST /api/bt/{npc}/{name} with JSON body', async () => {
    const treeJson = { version: '1.0.0', root: { id: 'r', type: 'sequence' } };
    const mockPayload = {
      npc_id: NPC,
      name: TREE,
      tree_json: treeJson,
      version: 2,
    };
    const fetchMock = mockFetchOnce(
      new Response(JSON.stringify(mockPayload), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    );

    const saved = await saveBtTree(NPC, TREE, treeJson);

    expect(saved).toEqual(mockPayload);
    expect(fetchMock).toHaveBeenCalledOnce();
    const [url, init] = fetchMock.mock.calls[0] as unknown as FetchArgs;
    expect(url).toBe(`/api/bt/${NPC}/${TREE}`);
    expect(init?.method).toBe('POST');
    // Body is the raw BTJSON document (proxy/bt-editor-api unwrap it)
    expect(JSON.parse(init?.body as string)).toEqual(treeJson);
  });

  it('saveBtTree → throws on 4xx', async () => {
    mockFetchOnce(
      new Response(JSON.stringify({ error: 'invalid tree' }), {
        status: 400,
        headers: { 'Content-Type': 'application/json' },
      }),
    );

    await expect(saveBtTree(NPC, TREE, {} as never)).rejects.toThrow(/400/);
  });

  it('simulateBtTree → POST /api/bt/{npc}/{name}/simulate and returns trace', async () => {
    const traceEntry = {
      tick: 1,
      node_id: 'r',
      status: 'success',
      message: 'ok',
    };
    const mockPayload = {
      npc_id: NPC,
      name: TREE,
      final_status: 'success',
      trace: [traceEntry],
    };
    const fetchMock = mockFetchOnce(
      new Response(JSON.stringify(mockPayload), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    );

    const result = await simulateBtTree(NPC, TREE, {
      world_state: { player_pos: [0, 0] },
    });

    expect(result).toEqual(mockPayload);
    const [url, init] = fetchMock.mock.calls[0] as unknown as FetchArgs;
    expect(url).toBe(`/api/bt/${NPC}/${TREE}/simulate`);
    expect(init?.method).toBe('POST');
  });
});
