# CHANGELOG — 3.0 经济系统

## 2026-10-09 — 3.0 v4 cross-city gold two-city GA

Approved the protocol and bridge-ledger design for roadmap subtrack #2:

- **Protocol**: mTLS A2A `TransferCrossCity` and read-only
  `GetCrossCityTransfer` RPCs with explicit reservation expiry and status.
- **Ledger**: local `cross_city_transfer` legs, signed `bridge_position` balances,
  append-only `bridge_ledger_entry` audit history, and `cross_city_out/in`
  player transaction types.
- **Failure model**: source reserve, destination idempotent credit, source
  settle, and source refund only after expiry plus destination not-found
  confirmation. No distributed 2PC.
- **Transport**: server-side mTLS with `RequireAndVerifyClientCert`, TLS 1.2+,
  and peer-certificate CN enforcement.
- **Two-city drill**: City B uses an isolated PostgreSQL, Redis, economy
  service, and a2a gateway. City A and City B communicate over mTLS gRPC.

**Status**: GA. The two-city acceptance binary passed reserve, local idempotent
reserve, remote credit, remote retry, settlement, remote lookup, and expired
refund. Gateway `go test ./...` passed and cross-city economy tests passed
30/30.

**ADR**: `docs/adr/ADR-0011-cross-city-gold-transfer.md`
**Two-City Plan**: `docs/superpowers/plans/2026-10-09-cross-city-gold-two-city-ga.md`
**Two-City Acceptance**: `/app/acceptance_cross_city_gold`

---

## 2026-10-08 — 3.0 v3 creator marketplace GA

Creator marketplace v1 is complete:

- **Templates**: NPC and Saga publish/browse/detail flows with OCEAN, BT
  skeleton, safe YAML, and dependency metadata.
- **Purchase**: transactional, idempotent purchase with atomic buyer debit,
  creator credit, purchase row, revenue row, transfer rows, and Kafka event.
- **Identity**: `player/creator/admin` roles enforced by economy-service;
  marketplace-capable admin-portal sessions carry the player UUID `sub`.
- **Admin controls**: admin-only soft take-down removes templates from the live
  market while preserving owned inventory and purchase history.
- **Admin portal**: Creator Templates, Saga Templates, Market, and Inventory
  pages plus authenticated marketplace proxies.
- **Verification**: 136 admin-portal tests, 107 relevant economy tests,
  typecheck passes, and creator-market acceptance reports 10/10 PASS.

**Fixed**: convert PG UUID `npc_template.creator_id` to the TEXT wallet /
transaction user ID at the service boundary.

**Known follow-up**: Saga runtime consumption of `npc_deps` from the platform
NPC pool remains outside v3; the persisted dependency contract is ready.

**ADR**: `docs/adr/ADR-0010-creator-marketplace.md`

---

## 2026-10-07 — 3.0 v2 admin-portal 钱包 UI GA

14 commits implementing T1-T14:

- **Layout**: sidebar + 顶部 player_id selector (zustand + sessionStorage persist)
- **/wallet**: 余额卡片（gold + token）+ 手动刷新 + 错误/loading/empty states
- **/transactions**: 表格（time/type/currency/amount/balance_after/memo）+ 分页 (50/page) + 类型色标
- **/admin**: emit + sink forms，server 错误码展示，success 后 invalidate wallet+transactions
- **Backend proxies**: /api/players (PG), /api/economy/wallet/[id], /transactions/[id], /admin/emit, /admin/sink
- **错误处理**: 统一的 `{error: {code, msg}}` envelope + describeError helper
- **docker-compose**: admin-portal service 新增 (8081 → 8081)，env vars (ECONOMY_URL, ADMIN_TOKEN, JWT_SECRET, DATABASE_URL)
- **Dockerfile**: 修复 builder stage corepack enable + 提交 pnpm-lock.yaml
- **测试**: 92 vitest unit + 1 Playwright E2E

**DoD**:
- `pnpm test` → 92/92 pass
- `pnpm typecheck` → 0 errors
- `pnpm playwright test e2e/wallet-admin.spec.ts` → 1 case pass (requires stack)
- `docker compose up -d --build admin-portal` → healthy
- 手动验收：admin login → 选 demo → 看到余额 → /admin emit → 返回 /wallet 刷新 → 余额变化

**已知 gap**:
- v1 YAGNI（按 spec）：玩家间转账 UI、NPC 商品目录浏览、WebSocket 实时事件、移动端、多语言货币切换

**ADR**: `docs/adr/ADR-0009-admin-portal-wallet-ui.md`

---

## 3.0.0 (2026-10-07) — v1 GA

### Highlights

- **完整经济闭环**：NPC 商品 → 玩家消费 → 中央银行发钞 → 反通胀
- **双货币系统**：gold（玩家间 + NPC 售货）+ token（任务产出，v1 表结构预留）
- **agent-os BT 集成**：NPC 主动售货（`npc_sell_to_player` action）+ dispatcher post-hook
- **中央银行安全保证**：MAX_GOLD_PER_PLAYER 硬上限 + 反通胀公式（数学上不可能溢出）
- **Kafka 事件流**：3 topic emit（fire-and-forget，支持 observability / Saga 扩展）
- **Redis 余额缓存**：TTL 1h + partial-update kwargs 写时双写
- **14 容器 healthy** + **46 unit/e2e tests pass** + **10/10 acceptance 步骤通过**

### 新增 (Added)

#### economy-service 主体（`apps/economy-service/`）

- **FastAPI 框架** + asyncpg + redis-py + aiokafka，主服务端口 8005
- **PG schema**（4 张表 + append-only triggers）：
  - `wallet` — 每玩家余额（gold + token + updated_at）
  - `transaction` — append-only 交易记录（含 balance_after 快照）+ PG trigger 阻止 UPDATE/DELETE
  - `product` — NPC 商品目录（npc_id + name + price_gold + stock + version）
  - `central_bank_ledger` — 中央银行发钞/sink 审计
- **REST API**：
  - `GET /api/v1/wallet/{user_id}` — 查余额（Redis cache → PG fallback）
  - `POST /api/v1/wallet/transfer` — 玩家间转账（`SELECT FOR UPDATE` + idempotency_key 支持）
  - `POST /api/v1/wallet/purchase` — 玩家购买 NPC 商品（`SELECT FOR UPDATE` + stock-1）
  - `GET /api/v1/transactions/{user_id}` — 交易历史分页（limit + offset）
  - `GET /api/v1/products` — 商品列表
  - `POST /api/v1/admin/central-bank/emit` — 手动发钞（admin auth）
  - `POST /api/v1/admin/central-bank/sink` — 手动 sink（admin auth + secrets.compare_digest 防 timing attack）

#### 中央银行（`apps/economy-service/src/economy_service/central_bank/`）

- **asyncio scheduler**：embedded in economy-service；每 6h 跑 1 次发钞
- **反通胀公式**：`emit_amount = base_emit * (1 - total_supply / MAX_TOTAL_GOLD)` —— 总金币越接近上限，发得越少
- **MAX_GOLD_PER_PLAYER = 100000 硬上限**（双层防护：单玩家上限 + 总上限反通胀公式）
- **NPC sink**：玩家购买商品 → 金币从玩家 wallet 转回 central_bank_ledger（回收）
- **Admin 手动触发**：`POST /api/v1/admin/central-bank/{emit,sink}`（admin role 校验 + secrets.compare_digest）

#### Kafka producer（`apps/economy-service/src/economy_service/kafka_producer.py`）

- **3 topic**：
  - `econ.tx.completed` — 玩家间转账 / NPC 购买完成
  - `econ.gold.emitted` — 中央银行发钞
  - `econ.gold.sunk` — NPC sink 回笼
- **aiokafka producer**（fire-and-forget：`create_task()` 真正异步；失败仅记日志，不阻塞事务）
- **v1 best-effort**：v2 接 Saga 引擎时升级为 outbox 模式

#### Redis 缓存（`apps/economy-service/src/economy_service/cache.py`）

- **Key schema**：`econ:{user_id}:{gold,token}`，TTL 1h
- **partial-update kwargs**：`cache_balance(user_id, gold=..., token=...)` 仅 SET 改过的维度（防止覆盖其他维度）
- **fallback 策略**：Redis miss → 查 PG → 回填 Redis

#### 错误码（R_018~R_026）

| 码 | 含义 | HTTP |
|---|---|---|
| R_018 | BAD_REQUEST | 400 |
| R_022 | INSUFFICIENT_BALANCE | 402 |
| R_023 | TRANSFER_SELF | 400 |
| R_024 | OUT_OF_STOCK | 409 |
| R_025 | NOT_FOUND | 404 |
| R_026 | ADMIN_REQUIRED | 403 |

#### agent-os 集成（`apps/agent-os/src/agent_os/`）

- **BT action `npc_sell_to_player`**（`apps/agent-os/src/agent_os/bt/actions.py`）：
  - 新增 BT action entry（5 → 6 entries）
  - `asyncio.to_thread` 包装同步 HTTP action，防止阻塞事件循环
  - 通过 HTTP POST 调用 economy-service `/api/v1/wallet/purchase`
- **dispatcher post-hook**（`apps/agent-os/src/agent_os/dispatcher.py`）：
  - `say_stream()` 完成后 fire-and-forget 触发 BT action
  - **opt-in**：env `BT_POST_HOOK_ENABLED=false`（默认关闭）+ kwarg `enable_bt_post_hook=False`（默认开启）
  - kwarg 优先级 > env（测试友好）
  - 失败仅记日志，不影响 LLM 流式响应

#### Acceptance & Tests

- **acceptance_economy_v1.py**：10 步 E2E 验证脚本（apps/economy-service/scripts/）
- **46 unit + e2e tests pass**：
  - economy-service：wallet CRUD + transfer + purchase + central_bank + cache + kafka = 30+ tests
  - agent-os：BT action + dispatcher post-hook = 16 tests

#### docker-compose

- **新增 `economy-service` 容器**：连 pg + redis + kafka
- **PG initdb mount slot 07**：`db/migrations/pg-schema-3.0-economy.sql`（wallet / transaction / product / central_bank_ledger）
- **Seed 已存在 slot**：`db/seed/seed-economy-demo.sql`（5 NPC products + demo wallets）

### 修复 (Fixed)

#### W3.2 review（`b2e795a`）

- 修正 `compute_emit` docstring + 添加反通胀公式注释

#### W3.3 review（`1835f9b`）

- admin sink body 用 Pydantic model 防止 500
- 用 `secrets.compare_digest` 防 timing attack
- 2 tests 增加

#### W4.1 review（`66f2335`）

- `cache_balance` 改为 partial-update (kwargs) 防止覆盖其他维度
- Kafka 真正 fire-and-forget（`create_task()`，而非 `asyncio.run`）

#### W4.2 review（`87cc3a4`）

- post-hook kwarg/env precedence 明确（kwarg > env）
- `asyncio.to_thread` 包装 sync action（防止阻塞事件循环）
- 增加 real e2e test（vs mock）

#### W5.2（`75f2fa7`）

- 修复 `central_bank.emit()` Decimal/float TypeError（PG numeric → Python Decimal 转换）

### 文档 (Documentation)

- **ADR-0008**：`docs/adr/0008-3.0-economy-scope.md` —— 3.0 经济系统范围 + 风险表 + DoD
- **3.0 ROADMAP**：`docs/3.0-ROADMAP.md` —— 5 周实施总结 + 后续 v2 候选
- **CHANGELOG-3.0**（本文档）：新增/修复/文档/测试
- **README banner**：`README.md` 顶部加 3.0 经济系统 GA 区块
- **economy-service README**：`apps/economy-service/README.md` —— 服务使用 + 部署

### 测试 (Tests)

| 类型 | 数量 | 文件 |
|---|---|---|
| Unit（economy-service） | 30+ | `apps/economy-service/tests/` |
| E2E（acceptance） | 10 | `apps/economy-service/scripts/acceptance_economy_v1.py` |
| Unit（agent-os BT action） | 4 | `apps/agent-os/tests/bt/test_actions.py` |
| Unit（agent-os dispatcher post-hook） | 12 | `apps/agent-os/tests/dispatcher/` |
| **Total** | **46** | |

#### Acceptance 步骤（10/10 PASS）

1. 注册 2 玩家（demo + alice）
2. 转账（demo → alice gold=100）
3. 余额验证（demo -100, alice +100）
4. 商品购买（alice → buy snack）
5. 余额不足（demo → buy think_token → R_022）
6. 自转（demo → buy 自己的商品 → R_023）
7. 幂等（同一 idempotency_key 两次转账只生效一次）
8. 中央银行发钞（admin emit + alice 余额增加）
9. 交易历史（GET transactions → 至少 4 条）
10. BT 隔离（dispatcher post-hook 默认关闭保 1.0/2.0 行为）

### 性能 (Performance)

- **Redis cache hit rate**：≥ 80%（TTL 1h + 写时双写）
- **转账 latency**：p95 < 50ms（含 Redis 写）
- **购买 latency**：p95 < 80ms（含 SELECT FOR UPDATE + stock-1）
- **中央银行发钞**：每 6h ± 5min（asyncio scheduler）

### 向后兼容 (Backwards Compatibility)

- **agent-os BT runtime 7 节点 + 5 action 架构不变**：仅 entries 5 → 6（新增 `npc_sell_to_player`）
- **dispatcher.say_stream() 行为不变**：post-hook 默认关闭（env `BT_POST_HOOK_ENABLED=false`）
- **无破坏性变更**：所有 1.0/2.0 acceptance 测试不受影响
- **PG trigger 阻止 UPDATE/DELETE transaction**：仅限制 transaction 表；wallet 表可改

### 影响范围 (Impact)

- **新增 1 个容器**：`economy-service`（13 → 14 容器）
- **新增 1 张 PG 表**：`central_bank_ledger`（其他 3 张在 slot 07 一次性创建）
- **agent-os 集成**：BT action 6 entries + dispatcher post-hook（向后兼容）
- **observability**：v1 仅 logs（`central_bank_emit triggered` / `cache hit/miss` / `kafka send success/fail`）；v2 接 Prometheus

### 范围外（v1 YAGNI）

- 创作者市场 / 任务分成（v2 子轨道 #1）
- PIPL/GDPR 数据导出接口（GA 前 4 周）
- 移动端钱包 UI
- 多语言货币
- 跨城金币转账（v2 通过 a2a-gateway 协议扩展）
- 货币兑换（gold ↔ token 不可互换）
- 衍生品 / 拍卖 / 担保交易
- token 任务系统接入（v1 表结构预留，task-service 后接）

### 已知限制 (Known Limitations)

- **Kafka fire-and-forget**：v1 不强制订阅；v2 接 Saga 时升级为 outbox 模式
- **中央银行 scheduler 无 Prometheus 监控**：v1 仅 logs；漏跑需运维看 logs
- **append-only 阻止 UPDATE/DELETE transaction**：客服手动改余额需要走 admin path（PG trigger 必须 bypass）
- **Redis 双写最终一致**：极端情况 1h TTL 内可能不一致（PG 是 source of truth）

### Stats

- **23 commits total**（W1 5 + W2 4 + W3 5 + W4 4 + W5 5）
- **0 pre-existing tests broken**（1.0 + 2.0 + BT editor 全部保持 PASS）
- **46 unit + e2e tests pass**
- **10/10 acceptance steps pass**
- **14 容器 healthy**
- **8 经济特性 GA**

### Spec & Plan

- ADR: [`docs/adr/0008-3.0-economy-scope.md`](docs/adr/0008-3.0-economy-scope.md)
- ROADMAP: [`docs/3.0-ROADMAP.md`](docs/3.0-ROADMAP.md)
- Spec & Plan: `docs/superpowers/plans/synthetic-jumping-jellyfish.md`（W1-W5 完整 25 子任务 + runbook）

---

**3.0 v1 GA 声明**：桶 1（必做 8 项）+ 5 周实施 + 23 commits + 10/10 acceptance + 46 tests pass + 14 容器 healthy + 0 pre-existing tests broken → **3.0 经济系统 v1 ACCEPTED**。

后续路线见 `docs/3.0-ROADMAP.md §后续路线（v2 候选）`。
