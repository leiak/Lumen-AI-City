/**
 * API 客户端封装。
 */
const API_BASE = process.env.NEXT_PUBLIC_API_GATEWAY || 'http://localhost:8080';

// 字段与 apps/world-engine/src/tile.rs::Tile 一致
export type LodLevel = 'CBD' | 'Residential' | 'Suburb';
export type BuildingKind = 'Tavern' | 'Plaza' | 'House' | 'Shop' | 'Park' | 'Road' | 'Office';

export interface Building {
  id: string;
  kind: BuildingKind;
  /** tile-local 坐标 (0..100)；渲染时需加上 (center_x - 50, center_y - 50) 转为世界坐标 */
  polygon: [number, number][];
}

export interface Tile {
  id: string;
  center_x: number;
  center_y: number;
  size: number;
  buildings: Building[];
  npc_ids: string[];
  player_ids: string[];
  lod_level: LodLevel;
}

export interface MoveResponse {
  player_id: string;
  current_tile_id: string;
  x: number;
  y: number;
  ts_ms: number;
  accepted: boolean;
  sequence?: number;
  source_channel?: string;
}

class ApiClient {
  private token: string | null = null;

  setToken(token: string) {
    this.token = token;
  }

  private async request<T>(path: string, options: RequestInit = {}): Promise<T> {
    const headers: HeadersInit = {
      'Content-Type': 'application/json',
      ...(this.token ? { Authorization: `Bearer ${this.token}` } : {}),
      ...(options.headers ?? {}),
    };
    const resp = await fetch(`${API_BASE}${path}`, { ...options, headers });
    if (!resp.ok) {
      throw new Error(`API ${resp.status}: ${await resp.text()}`);
    }
    return resp.json();
  }

  // 字段与 apps/api-gateway/internal/handlers/auth.go 的 loginResponse 一致
  login = (username: string, password: string) =>
    this.request<{
      token: string;
      player_id: string;
      username: string;
      display_name: string;
      expires_at: string;
    }>('/v1/auth/login', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
    });

  // GET /v1/tiles → 9 个 tile (api-gateway 反代到 world-engine REST)
  // 字段定义见 apps/world-engine/src/tile.rs::Tile
  getTiles = () => this.request<Tile[]>('/v1/tiles');

  // POST /v1/world/move
  // 字段定义见 apps/api-gateway/internal/handlers/world_move.go::moveRequestBody
  move = (params: {
    player_id: string;
    from_tile_id: string;
    to_tile_id: string;
    x: number;
    y: number;
  }) =>
    this.request<MoveResponse>('/v1/world/move', {
      method: 'POST',
      body: JSON.stringify(params),
    });

  getNpc = (id: string) => this.request<unknown>(`/v1/npcs/${id}`);

  dialogue = (npcId: string, message: string) =>
    this.request<{ reply: string }>(`/v1/npcs/${npcId}/dialogue`, {
      method: 'POST',
      body: JSON.stringify({ message }),
    });
}

export const api = new ApiClient();

// Module load 时同步 token（login/page.tsx 已写入 localStorage，刷新页面后即此路径补回）。
// 必须在 export api 之后；浏览器 SSR 安全（typeof window 守卫）。
if (typeof window !== 'undefined') {
  const t = window.localStorage.getItem('aicity_token');
  if (t) api.setToken(t);
}
