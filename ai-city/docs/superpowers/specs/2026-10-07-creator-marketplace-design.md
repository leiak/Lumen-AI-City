# 3.0 v3 创作者市场 — 设计 Spec

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:writing-plans to convert this design into an implementation plan. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **Status:** Approved (2026-10-07)
> **ADR:** TBD (ADR-0010)
> **Roadmap:** `docs/3.0-ROADMAP.md` §后续路线 (v3 候选)
>
> ## 用户决策汇总
> - **MVP 范围**: 完整三件套 (NPC 模板 + Saga 模板 + 分成) — 8 周 scope
> - **身份模型**: 复用 player 表，加 role='creator'/'admin'（3 个 role: player / creator / admin）
> - **发布流**: 创作者发布即上架；admin 可下架（无强制审核）
> - **分成模型**: 创作者全拿，平台记账（commission=0，未来字段预留）
> - **跨城**: 全平台市场（无 per-city curation）
> - **货币**: 复用 gold（不引入 gem）
> - **NPC 模板内容**: 人格（OCEAN + 头像 + 名字）+ BT 骨架 + 商品目录 + 价格
> - **Saga 模板内容**: .yaml + name + icon + description + NPC deps + semantic_version
> - **创作者收入入账**: 购买即时入账
> - **平台 commission**: v1 = 0，admin 后台可配置（未来）
> - **Saga deps**: 运行时从 platform_npc_pool 拉公共 NPC（玩家不需购买）
> - **架构**: A — 扩 economy-service + admin-portal（PG slot 08）

## 目标

让"创作者"（运营/玩家/第三方）能创作者在 AI 城邦生态里发布自己的 NPC 模板（人格+BT+商品）和 Saga 模板（脚本+依赖），玩家用 gold 购买，创作者即时收到分成。Stakeholder 看到"NPC 商店 + Saga 模板市场 + 创作者经济"完整闭环。

承接 3.0 v1（economy-service + gold）+ 3.0 v2（admin-portal 钱包 UI），从"经济系统"升级为"创作者经济"。

## 范围

### 在 v1 范围内

- 双模板市场：NPC 模板 + Saga 模板
- 3 角色身份：player（默认）/ creator（创作者后台）/ admin（公权下架）
- 创作者发布流：表单编辑 → 一键上架
- 玩家购买流：浏览 /market / → 详情页 → 支付 gold
- 创作者即时收入：购买即时入账（gold 余额增加）
- admin 下架权：marketplace 后台即时隐藏
- Saga runtime 依赖：platform_npc_pool 公共 NPC 库（玩家不需购买）
- 8 步 acceptance：`acceptance_creator_market_v1.py`

### 显式不做（v1 YAGNI）

- ❌ 模板版本升级对老买家的兼容（v1 单版本号）
- ❌ 创作者上传 avatar 图片（只接 URL）
- ❌ 多人协同编辑模板
- ❌ 模板 fork（v2 candidate）
- ❌ 跨城分成结算（v1 全平台同账）
- ❌ 模板统计报表（v1 仅 keep revenue ledger）
- ❌ 平台 commission（v1 字段预留=0，admin 后台未来配置）
- ❌ 创作者评级 / 评论系统
- ❌ 第三方支付（仅 game 内 gold）

## 架构

```
┌─────────────────────────────────────────────────────┐
│ admin-portal (:8081)                              │
│ ├─ /creator/npc-templates     (creator 后台)    │
│ ├─ /creator/saga-templates    (creator 后台)    │
│ ├─ /market                     (marketplace 浏览)│
│ ├─ /market/npc-templates/{id} (NPC 详情 + 购买)  │
│ ├─ /market/saga-templates/{id} (Saga 详情 + 购买) │
│ └─ /inventory                  (我购买的模板)    │
│ │
│ /api/* → admin-portal → economy-service │
│           (JWT + role)   (brokerage + PG slot 08) │
└─────────────────────────────────────────────────────┘
                          │
                ┌─────────▼─────────┐
                │ economy-service  │
                │     (:8005)      │
                │ + 9 new endpoints│
                │ + PG slot 08     │
                └────────────────┘
                          │
                Kafka     │ emit market.purchased
                          ▼
                  observability + analytics
```

### 关键设计决策

1. **3-role 身份模型**: `player.role` 加列（`player`/`creator`/`admin`）；admin-portal middleware（已存在）检查 JWT `role` claim。**YAGNI 不做独立 creator 表 / RBAC**。
2. **NPC 模板 = 人 + BT + 商品**: name + avatar_url + OCEAN 5 维 + BT skeleton JSON + product_catalog + price_gold。
3. **Saga 模板 = yaml + metadata**: yaml_content + name + icon_url + description + npc_deps[] + semantic_version。
4. **publish 即上架**: 创作者点发布 → status='live' → market 可见；admin 可下架 status='taken_down'。
5. **即时分成**: 购买 → PG 事务：扣玩家 gold → 写 template_purchase → 写 creator_revenue → 加 creator gold。**imempotency_key 防重**。
6. **Saga 依赖解析**: saga-worker 启动 Saga 时，遍历 npc_deps，从 platform_npc_pool（全局只读 view）拉取 NPC 实例，玩家不需购买依赖。
7. **commission 字段预留**: v1 platform_cut_gold = 0；表 schema 预留字段，避免 schema 漂移。
8. **global market**: 不做 per-city curation，所有玩家可见所有 live 模板。

## 数据模型（PG migration slot 08）

```sql
-- player.role 扩展（slot 08 增量）
ALTER TABLE player ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT 'player'
  CHECK (role IN ('player', 'creator', 'admin'));

-- NPC 模板
CREATE TABLE npc_template (
    id              BIGSERIAL PRIMARY KEY,
    creator_id      TEXT NOT NULL REFERENCES player(id) ON DELETE RESTRICT,
    name            TEXT NOT NULL,
    avatar_url      TEXT,
    ocean_json      JSONB NOT NULL,
    bt_skeleton     TEXT,
    product_catalog JSONB,
    price_gold      BIGINT NOT NULL CHECK (price_gold >= 10),
    status          TEXT NOT NULL DEFAULT 'live' CHECK (status IN ('live', 'taken_down')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_npc_template_creator ON npc_template(creator_id);
CREATE INDEX idx_npc_template_status ON npc_template(status) WHERE status='live';

-- Saga 模板
CREATE TABLE saga_template (
    id              BIGSERIAL PRIMARY KEY,
    creator_id      TEXT NOT NULL REFERENCES player(id) ON DELETE RESTRICT,
    name            TEXT NOT NULL,
    icon_url        TEXT,
    description     TEXT,
    yaml_content    TEXT NOT NULL,
    npc_deps        TEXT[] NOT NULL DEFAULT '{}',
    semantic_version TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'live' CHECK (status IN ('live', 'taken_down')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_saga_template_creator ON saga_template(creator_id);
CREATE INDEX idx_saga_template_status ON saga_template(status) WHERE status='live';

-- 购买记录
CREATE TABLE template_purchase (
    id              BIGSERIAL PRIMARY KEY,
    user_id         TEXT NOT NULL REFERENCES player(id) ON DELETE CASCADE,
    template_kind   TEXT NOT NULL CHECK (template_kind IN ('npc', 'saga')),
    template_id     BIGINT NOT NULL,
    price_paid_gold BIGINT NOT NULL,
    idempotency_key TEXT UNIQUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_purchase_user ON template_purchase(user_id);

-- 创作者收入 ledger
CREATE TABLE creator_revenue (
    id              BIGSERIAL PRIMARY KEY,
    creator_id      TEXT NOT NULL REFERENCES player(id) ON DELETE RESTRICT,
    purchase_id     BIGINT NOT NULL REFERENCES template_purchase(id),
    amount_gold     BIGINT NOT NULL,
    platform_cut_gold BIGINT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_creator_revenue_creator ON creator_revenue(creator_id, created_at DESC);
```

### 设计要点

- `npc_template.ocean_json`: 必填 5 维浮点 [0, 1]，对应现有 `OceanPersonality`
- `npc_template.bt_skeleton`: 可空 JSON 字符串；非空时 Pydantic 校验 + depth ≤ 10 + 节点数 ≤ 50
- `npc_template.product_catalog`: 可空 JSON 数组，每项 `{name, price_gold, stock?}`
- `saga_template.yaml_content`: 文本，由 server 侧 sandbox 解析（禁用 `!!python/object` 等危险 tag）
- `saga_template.npc_deps`: 引用 npc_template.id 的字符串列表（不强制 ON DELETE CASCADE，留独立空间）
- `template_purchase.idempotency_key`: UUID UNIQUE 防重（沿用 3.0 v1 模式）
- `creator_revenue`: 平台记账字段预留（v1 platform_cut_gold=0）

## REST API (economy-service +9 endpoints)

| Method | Path | Auth | Body | 返回 |
|---|---|---|---|---|
| POST | `/v1/marketplace/npc-templates` | creator | `{name, avatar_url?, ocean_json, bt_skeleton?, product_catalog?, price_gold}` | `{id}` |
| GET | `/v1/marketplace/npc-templates` | public | `?status=live&limit&offset` | `[{...}]` |
| GET | `/v1/marketplace/npc-templates/{id}` | public | — | `{...}` |
| POST | `/v1/marketplace/npc-templates/{id}/take-down` | admin | — | `200` |
| POST | `/v1/marketplace/saga-templates` | creator | `{name, icon_url?, description?, yaml_content, npc_deps?, semantic_version}` | `{id}` |
| GET | `/v1/marketplace/saga-templates` | public | `?status=live&limit&offset` | `[{...}]` |
| POST | `/v1/marketplace/purchase` | player | `{template_kind, template_id, idempotency_key}` | `{purchase_id, price_paid, creator_id}` |
| GET | `/v1/marketplace/inventory/{user_id}` | player | — | `[owned templates]` |
| GET | `/v1/marketplace/revenue/{creator_id}` | creator | — | `[{revenue entries}]` |

### 新增错误码

| 码 | 含义 | HTTP |
|---|---|---|
| R_027 | CREATOR_REQUIRED | 403 |
| R_028 | TEMPLATE_NOT_FOUND | 404 |
| R_029 | TEMPLATE_TAKEN_DOWN | 410 |
| R_030 | PRICE_INVALID | 400 |
| R_031 | BT_INVALID | 400 |
| R_032 | YAML_INVALID | 400 |
| R_033 | INSUFFICIENT_BALANCE | 402 — 沿用 R_022 |
| R_034 | PURCHASE_DUPLICATE | 409 |

### Body 形态

```json
// POST /v1/marketplace/npc-templates
{
  "name": "Chef Wang",
  "avatar_url": "https://...",
  "ocean_json": {"O": 0.7, "C": 0.8, "E": 0.5, "A": 0.6, "N": 0.3},
  "bt_skeleton": "{\"type\":\"sequence\",\"children\":[...]}",  // optional
  "product_catalog": [{"name": "noodle", "price_gold": 50, "stock": null}],
  "price_gold": 100
}

// POST /v1/marketplace/saga-templates
{
  "name": "Welcome Party",
  "icon_url": "https://...",
  "description": "3 NPC welcome newbies",
  "yaml_content": "saga:\n  name: welcome\n  steps:\n    - task: hello\n      npc: chef_wang\n",
  "npc_deps": ["1", "2"],
  "semantic_version": "1.0.0"
}

// POST /v1/marketplace/purchase
{
  "template_kind": "npc",
  "template_id": 42,
  "idempotency_key": "uuid-v4"
}
```

### 幂等性

`POST /v1/marketplace/purchase` 接受 `idempotency_key` UUID；同一 key 重复请求返回首次结果（沿用 3.0 v1 模式）。

## UI 详细（admin-portal）

### `/creator/npc-templates` — 列表页

- 卡片网格：name + 价格 + status badge（live/taken_down）+ 编辑按钮
- 顶部 [创建 NPC 模板] 按钮

### `/creator/npc-templates/new` — 编辑表单

```tsx
<form>
  <input name="name" placeholder="NPC 名字" />
  <input name="avatar_url" placeholder="头像 URL" />
  <input name="price_gold" type="number" min={10} />
  <fieldset>
    <legend>OCEAN 人格</legend>
    <Slider label="Openness" min={0} max={1} step={0.1} />
    <Slider label="Conscientiousness" min={0} max={1} step={0.1} />
    <Slider label="Extraversion" min={0} max={1} step={0.1} />
    <Slider label="Agreeableness" min={0} max={1} step={0.1} />
    <Slider label="Neuroticism" min={0} max={1} step={0.1} />
  </fieldset>
  <textarea name="bt_skeleton" placeholder="BT JSON（可选）" />
  <fieldset>
    <legend>商品目录</legend>
    <input name="product_name" />
    <input name="product_price" type="number" />
    [添加商品] 按钮
  </fieldset>
  [保存草稿] [发布到市场]
</form>
```

### `/market` — 浏览页

- 顶部 tab：NPC 模板 / Saga 模板
- 卡片：name + avatar + creator + price + [详情]
- 右上搜索框

### `/market/npc-templates/{id}` — 详情页

- 顶部：name + avatar + creator name
- 中部：OCEAN radar chart（5 维 SVG）
- 下部：BT skeleton（仅展示，不可改）+ 商品 list
- 右侧栏：价格 + [购买] 按钮
- 若 creator 是当前用户：显示 [编辑] [下架]

### `/inventory` — 我的库存

- 卡片网格：已购买的 NPC + Saga 模板
- NPC：可"应用至我创建的 NPC"（v1 仅展示）
- Saga：可"运行该 Saga"（v1 仅展示）

## 状态管理

### JWT role 扩展

```typescript
// lib/auth.ts (existing) — JWT payload 增加 role
interface JwtPayload {
  username: string;
  role: 'player' | 'creator' | 'admin';
  exp: number;
}
```

admin-portal middleware（已存在）解码 JWT → `role` 写入 cookie session → 路由的 `requireRole('creator'|'admin')` 守卫。

## 数据流

### 创作者发布 NPC 模板

1. creator 访问 `/creator/npc-templates/new`
2. 填表 → react-hook-form + zod 校验
3. POST `/api/marketplace/npc-templates` (admin-portal proxy 加 JWT)
4. economy-service 校验 role + 校验 Pydantic schema
5. INSERT INTO npc_template status='live'
6. 返回 `{id}` → 重定向到 `/creator/npc-templates`

### 玩家购买

1. player 访问 `/market/npc-templates/42`
2. 点"购买" → react-query mutation
3. POST `/api/marketplace/purchase` with idempotency_key
4. economy-service PG 事务:
   - SELECT FOR UPDATE player.wallet
   - 检查 balance >= price_paid_gold
   - 扣玩家 gold → 写 transaction(player_transfer, out)
   - INSERT INTO template_purchase(idempotency_key)
   - INSERT INTO creator_revenue(platform_cut=0)
   - SELECT FOR UPDATE creator.wallet
   - 加 creator gold → 写 transaction(player_transfer, in)
   - 双 Redis 写（v1 双键）
   - Kafka emit `market.purchased`
5. 返回 `{purchase_id, creator_id}` → invalidate `/inventory`

### Saga runtime 依赖解析

1. saga-worker 启动 Saga → 遍历 saga_template.npc_deps[]
2. 对每个 dep_id: SELECT FROM npc_template WHERE id=dep_id AND status='live' → 注入到 saga state
4. 若 dep 被下架：fail-fast + 错误日志（玩家无需重购，因为 platform_npc_pool 不存实例）

## 部署

### docker-compose.yml 改动

无需新容器；economy-service 已存在。Slot 08 mount。

## 测试策略

### 单元测试 (pytest, 30+)

- `test_marketplace_npc_template.py` — CRUD + role check + 校验（10）
- `test_marketplace_saga_template.py` — CRUD + YAML 解析 sandbox（10）
- `test_marketplace_purchase.py` — 余额扣款 + idempotency + revenue 记入（10+）

### 集成测试 (pytest, 8+)

- `test_e2e_npc_purchase.py` — 创作者发 → 玩家买 → 创作者收 → 库存增加
- `test_e2e_saga_purchase.py` — Saga 模板发布 → 玩家购买 → runtime 拉 deps
- `test_e2e_takedown.py` — admin 下架 → market 消失

### E2E acceptance (`scripts/acceptance_creator_market_v1.py`, 10 步)

1. 注册 creator 用户
2. creator 创建 NPC 模板 (price=100)
3. 玩家在 /market 看到模板
4. 玩家购买 → gold -100 → creator gold +100
5. creator 在 /creator/revenue 看到 +100
6. 玩家在 /inventory 看到已购
7. creator 创建 Saga 模板 (yaml + 引用 NPC)
8. saga-worker 加载 Saga → 拉 platform_npc_pool
9. admin 下架 → /market 中消失
10. saga 已售实例不受影响

### Vitest (admin-portal, 9+)

- `tests/creator/npc-template.test.tsx` (3)
- `tests/creator/saga-template.test.tsx` (3)
- `tests/market.test.tsx` (3)
- Playwright E2E 1 case

## 文档

- ADR-0010: 创作者市场 v1 — 3-role 身份 + PG slot 08 + economy-service 扩 + 创作者即分账
- `docs/3.0-ROADMAP.md` 加 §3.0 v3 创作者市场 section
- `CHANGELOG-3.0.md` 加 v3 GA entry

## 风险

| 风险 | 缓解 |
|---|---|
| BT skeleton JSON 注入恶意 schema | 严格 Pydantic 校验 + depth ≤ 10 + 节点数 ≤ 50 |
| Saga .yaml 含恶意引用 | server 侧 sandbox 解析，禁用 `!!python/object` 等危险 tag |
| 创作者把 price 改成 1 gold 套现 | price_min = 10 gold |
| 已售出 NPC 实例和模板脱钩 | 模板可被 takedown 但已售实例仍可用 |
| 创作者发布 spam 模板 | v1 不强制审核，admin 可批量下架 + future rate limit |
| gold 套现（刷单→提现） | idempotency_key + PG SELECT FOR UPDATE |
| Saga 依赖被下架 | saga-worker fail-fast + 错误日志（玩家不需重购） |

## Critical Files (pre-implementation)

### Create

- `packages/proto/pg-schema-3.0-creator-market.sql` — 4 表 + role 扩展
- `db/seed/seed-creator-market.sql` — seed 2 creator 用户 + 1 admin
- `apps/economy-service/src/economy_service/api/v1/marketplace/npc_templates.py`
- `apps/economy-service/src/economy_service/api/v1/marketplace/saga_templates.py`
- `apps/economy-service/src/economy_service/api/v1/marketplace/purchase.py`
- `apps/economy-service/src/economy_service/services/marketplace_service.py`
- `apps/economy-service/src/economy_service/services/template_validator.py`
- `apps/economy-service/scripts/acceptance_creator_market_v1.py`
- `apps/admin-portal/src/app/creator/npc-templates/page.tsx` + `NpcTemplateList.tsx`
- `apps/admin-portal/src/app/creator/npc-templates/new/page.tsx` + `NpcTemplateEdit.tsx`
- `apps/admin-portal/src/app/creator/saga-templates/page.tsx` + `SagaTemplateList.tsx`
- `apps/admin-portal/src/app/creator/saga-templates/new/page.tsx` + `SagaTemplateEdit.tsx`
- `apps/admin-portal/src/app/market/page.tsx` + `MarketClient.tsx`
- `apps/admin-portal/src/app/market/npc-templates/[id]/page.tsx` + `NpcTemplateDetail.tsx`
- `apps/admin-portal/src/app/market/saga-templates/[id]/page.tsx` + `SagaTemplateDetail.tsx`
- `apps/admin-portal/src/app/inventory/page.tsx` + `InventoryClient.tsx`
- `apps/admin-portal/src/app/api/marketplace/[...]/route.ts` — proxies (4 routes)
- `apps/admin-portal/src/lib/marketplace.ts` — typed helpers
- `apps/admin-portal/tests/creator/*.test.tsx` — 9 vitest
- `apps/admin-portal/e2e/creator-market.spec.ts` — Playwright 1

### Modify

- `apps/economy-service/src/economy_service/errors.py` — 加 R_027~R_034
- `apps/economy-service/src/economy_service/app.py` — 注册新 router
- `apps/admin-portal/src/lib/auth.ts` — JWT payload 增加 `role` claim
- `apps/admin-portal/src/middleware.ts` (if exists) — role check
- `apps/admin-portal/src/components/Sidebar.tsx` — 加 Creator + Market + Inventory 链接
- `docker-compose.yml` — slot 08 mount
- `docs/3.0-ROADMAP.md` — 加 §v3 创作者市场
- `CHANGELOG-3.0.md` — 加 v3 GA

### Read (existing)

- `apps/economy-service/src/economy_service/api/v1/` — v1 endpoints 模式
- `apps/economy-service/src/economy_service/services/wallet_service.py` — 事务模式
- `apps/admin-portal/src/lib/auth.ts` — JWT helpers
- `apps/admin-portal/src/app/wallet/WalletClient.tsx` — react-query 模式

## Acceptance (DoD)

1. `pnpm test` (admin-portal) → 9+ vitest pass
2. `pytest` (economy-service) → 30+ unit + 8+ e2e pass
3. `pnpm playwright test e2e/creator-market.spec.ts` → 1 case pass
4. `acceptance_creator_market_v1.py` → 10/10 PASS
5. `docker compose up -d --build` → all containers healthy (no new container — extends economy-service)
6. `pnpm typecheck` → 0 errors
7. 手动验收：
   - creator 用户创建模板 → /market 可见
   - 玩家购买 → gold 扣减 + 创作者 gold 增加
   - admin 下架 → /market 中消失
   - saga 已售实例正常加载依赖