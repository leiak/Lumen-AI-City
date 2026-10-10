/**
 * API 客户端封装。
 */
import type { NpcDialoguePayload } from './ws-events';

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

export interface NpcInfo {
  npc_id: string;
  name: string;
  home_tile_id: string;
  say: string;
  options: Array<{ id: string; text: string }>;
}

export interface MeResponse {
  player_id: string;
  username: string;
  display_name: string;
  avatar_url: string;
}


export interface WalletResponse {
  user_id: string;
  gold_balance: number;
  token_balance: number;
  created_at: string;
  updated_at: string;
}

export interface Product {
  id: number;
  npc_id: string;
  name: string;
  price_gold: number;
  price_token: number | null;
  stock: number | null;
  enabled: boolean;
}

export interface PurchaseResponse {
  user_id: string;
  product_id: number;
  currency: 'gold' | 'token';
  amount_paid: number;
  balance_after: number;
  sink_amount: number;
}

export interface Transaction {
  id: number;
  tx_type: string;
  currency: 'gold' | 'token';
  amount: number;
  balance_after: number;
  counterparty_id: string | null;
  product_id: number | null;
  trace_id: string | null;
  created_at: string | null;
}

export interface TransactionListResponse {
  transactions: Transaction[];
  total: number;
}

export interface InventoryItem {
  product_id: number;
  name: string;
  quantity: number;
  currencies: string | null;
  last_purchased_at: string | null;
}

export interface TransferResponse {
  user_id: string;
  gold_balance: number;
  token_balance: number;
  created_at: string;
  updated_at: string;
}

export interface NpcMarketTemplate {
  id: number;
  name: string;
  price_gold: number;
  creator_id: string;
  status: string;
}

export interface SagaMarketTemplate {
  id: number;
  name: string;
  price_gold: number;
  creator_id: string;
  description: string | null;
  status: string;
}

export interface MarketplacePurchaseResponse {
  purchase_id: number;
}

export interface CreatorRevenueItem {
  purchase_id: number;
  amount_gold: number;
  platform_cut_gold: number;
  created_at: string | null;
}

export interface CreatorRevenueSummary {
  earned_gold: number;
  withdrawn_gold: number;
  available_gold: number;
}

export interface CreatorRevenueWithdrawalResponse {
  withdrawal_id: number;
  amount_gold: number;
  balance_after: number;
  status: string;
}

export interface CreateTemplateResponse {
  id: number;
}

export interface CreateTemplateResponse {
  id: number;
}

export interface InventoryItem {
  product_id: number;
  name: string;
  quantity: number;
  currencies: string | null;
  last_purchased_at: string | null;
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

  getMe = () => this.request<MeResponse>('/v1/players/me');


  getWallet = () => this.request<WalletResponse>('/v1/wallet');

  listProducts = (npcId: string) =>
    this.request<Product[]>(`/v1/products/${encodeURIComponent(npcId)}`);

  purchaseProduct = (params: {
    productId: number;
    currency: 'gold' | 'token';
    idempotencyKey: string;
    traceId?: string;
  }) =>
    this.request<PurchaseResponse>('/v1/wallet/purchase', {
      method: 'POST',
      body: JSON.stringify({
        product_id: params.productId,
        currency: params.currency,
        idempotency_key: params.idempotencyKey,
        trace_id: params.traceId,
      }),
    });

  getTransactions = () =>
    this.request<TransactionListResponse>('/v1/transactions');

  getInventory = () => this.request<InventoryItem[]>('/v1/inventory');

  transfer = (params: {
    toUserId: string;
    amount: number;
    idempotencyKey: string;
    memo?: string;
  }) =>
    this.request<TransferResponse>('/v1/wallet/transfer', {
      method: 'POST',
      body: JSON.stringify({
        to_user_id: params.toUserId,
        currency: 'gold',
        amount: params.amount,
        idempotency_key: params.idempotencyKey,
        memo: params.memo,
      }),
    });

  listNpcTemplates = () =>
    this.request<NpcMarketTemplate[]>('/v1/marketplace/npc-templates');

  listSagaTemplates = () =>
    this.request<SagaMarketTemplate[]>('/v1/marketplace/saga-templates');

  purchaseMarketplaceTemplate = (params: {
    templateKind: 'npc' | 'saga';
    templateId: number;
    idempotencyKey: string;
  }) =>
    this.request<MarketplacePurchaseResponse>('/v1/marketplace/purchase', {
      method: 'POST',
      body: JSON.stringify({
        template_kind: params.templateKind,
        template_id: params.templateId,
        idempotency_key: params.idempotencyKey,
      }),
    });

  getCreatorRevenue = () =>
    this.request<CreatorRevenueItem[]>('/v1/marketplace/revenue');

  getCreatorRevenueSummary = () =>
    this.request<CreatorRevenueSummary>('/v1/marketplace/revenue-summary');

  withdrawCreatorRevenue = () =>
    this.request<CreatorRevenueWithdrawalResponse>(
      '/v1/marketplace/revenue-withdraw',
      {
        method: 'POST',
        body: JSON.stringify({
          idempotency_key: `creator-withdraw-${Date.now().toString(36)}`,
        }),
      },
    );

  createNpcTemplate = (params: {
    name: string;
    priceGold: number;
    ocean: { O: number; C: number; E: number; A: number; N: number };
  }) =>
    this.request<CreateTemplateResponse>('/v1/marketplace/npc-templates', {
      method: 'POST',
      body: JSON.stringify({
        name: params.name,
        price_gold: params.priceGold,
        ocean_json: params.ocean,
      }),
    });

  createSagaTemplate = (params: {
    name: string;
    priceGold: number;
    semanticVersion: string;
    description?: string;
    yamlContent: string;
    npcDeps: string[];
  }) =>
    this.request<CreateTemplateResponse>('/v1/marketplace/saga-templates', {
      method: 'POST',
      body: JSON.stringify({
        name: params.name,
        price_gold: params.priceGold,
        semantic_version: params.semanticVersion,
        description: params.description || undefined,
        yaml_content: params.yamlContent,
        npc_deps: params.npcDeps,
      }),
    });

  takeDownMarketTemplate = (params: { kind: 'npc' | 'saga'; templateId: number }) =>
    this.request<{ status: string }>(
      `/v1/marketplace/${params.kind === 'npc' ? 'npc' : 'saga'}-templates/${params.templateId}/take-down`,
      { method: 'POST' },
    );

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

  getNpc = (id: string) => this.request<NpcInfo>(`/v1/npcs/${id}`);

  // POST /v1/npc/:id/talk —— Sprint 12 spec endpoint（npc_id 在 URL 路径）
  // body 只携带 {player_id, choice_id}，npc_id 不再重复出现在 body 中。
  // 后端实现：apps/api-gateway/internal/handlers/npc_talk.go::HandleByID
  // 返回形状与 ws-gateway NpcDialoguePayload 一致（见下）。
  postNpcTalk = (npcId: string, choiceId: string, playerId: string) =>
    this.request<NpcDialoguePayload>(`/v1/npc/${npcId}/talk`, {
      method: 'POST',
      body: JSON.stringify({
        player_id: playerId,
        choice_id: choiceId,
      }),
    });

}

export const api = new ApiClient();

// Module load 时同步 token（login/page.tsx 已写入 localStorage，刷新页面后即此路径补回）。
// 必须在 export api 之后；浏览器 SSR 安全（typeof window 守卫）。
if (typeof window !== 'undefined') {
  const t = window.localStorage.getItem('aicity_token');
  if (t) api.setToken(t);
}
