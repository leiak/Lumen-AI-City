# 2.0.6-stage3-phase-c-bt-editor-admin-auth (2026-10-06)

## [Phase C] BT 编辑器 + admin auth — 2026-10-06

3-piece set 最后一块落子：BT 编辑器运行时 + 后端 + UI + admin 鉴权。
3-phase backlog (OCEAN→emotion / Saga viz / BT editor) 全部关闭。

### C.4 — admin auth（新增）
- **PG migration** `packages/proto/pg-schema-2.0-bt-auth.sql`：`ALTER TABLE player ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT 'player'` + 部分索引 + `CREATE EXTENSION IF NOT EXISTS pgcrypto`。Mount slot 05（idempotent）。
- **Seed** `db/seed/seed-admin.sql`：bcrypt(`adminpass`, gen_salt('bf')) + role='admin'；ON CONFLICT 更新密码。Mount slot 06。
- **`auth.ts`**：手写 HS256 JWT（仅 `node:crypto`，无新依赖）— `signToken` / `decodeToken`（alg pinning 拒 `alg=none`、timing-safe HMAC、30s clock skew、role 白名单）；导出 `COOKIE_NAME='aicity_token'` + `SESSION_MAX_AGE_SEC=7d`。
- **`middleware.ts`**：Edge runtime 守卫 `/bt-editor/:path*` + `/api/bt/:path*`；无 cookie → API 401 JSON `{detail:{code:'R_401'}}` / UI 303 → `/login?next=…`。
- **`/login` 页** + **`/api/auth/login`**（JSON + form 双模）+ **`/api/auth/logout`**；默认 `admin/adminpass`，env `ADMIN_USERNAME`/`ADMIN_PASSWORD` 覆盖；复用 `JWT_SECRET`（与 api-gateway/ws-gateway 一致）。
- **`/bt-editor` server component**：cookie 通过后再做 `decodeToken + isAdmin` 二次校验；非 admin 渲染友好 403。
- **主页 `/` nav**：登录态显示 username + 退出按钮；未登录显示「管理员登录」链接。

### C.4 — acceptance_bt_editor.py（新增）
- `apps/agent-os/scripts/acceptance_bt_editor.py` 7 步 E2E：list / get-missing / save-valid / save-invalid (R_019 400) / save-oversize (R_019 422 depth 12 > 10) / get-after-save / simulate。
- `--smoke` 模式：API 不通时 SKIP exit 0（CI 友好）。
- 零第三方依赖（urllib stdlib only）。

### 测试
- admin-portal vitest：**64/64 PASS**（含 14 auth 新增；C.3 阶段 50 → C.4 阶段 64）
- agent-os acceptance_bt_editor：**7/7**（docker compose up 时）
- 1 新 PG migration + 1 新 seed，全部 idempotent

### 关键变更
- `docker-compose.yml`：postgres initdb 多挂 2 个文件（slot 05/06）
- 8 个 admin-portal 新/改文件：`auth.ts` / `auth.test.ts` / `middleware.ts` / `login/page.tsx` / `api/auth/login/route.ts` / `api/auth/logout/route.ts` / `page.tsx` (nav) / `bt-editor/page.tsx` (server-side role check)
- 1 个 agent-os 新脚本：`acceptance_bt_editor.py`
- 2 个 PG SQL：migration + seed
- docs：`docs/2.0-ROADMAP.md` 新增 Phase C 章节 + status 行加 Phase C GA

### 影响
- **性能**：middleware 是 Edge runtime O(1) cookie 检查；server-side decode 仅在受保护路由首次访问时执行；登录是单次 bcrypt 比对（cost 10，~100ms） + 1 HMAC-SHA256 sign（<1ms）。
- **向后兼容**：现有 `demo/demo123` 玩家不受影响（role 默认 'player'）；`pgcrypto` 已在 `pg-schema.sql` 创建；C.1/C.2/C.3 代码冻结未改。
- **零回归**：C.3 阶段 50 vitest tests 全部 PASS；BT editor C.3 UI 不变，仅多了服务端 role 校验。

### 已知限制
- v0 登录是 env-常量校验（admin/adminpass）。生产部署应：
  1. 设 `ADMIN_USERNAME` + `ADMIN_PASSWORD` env
  2. 或挂 PG bcrypt 查询（admin-portal 起 pg 连接 + 验 player 表）
- 无 2FA / 无密码重置 / 无 audit log（YAGNI，阶段 4 backlog）
- middleware 在 Edge runtime 仅做 cookie 存在性检查，完整 JWT 验签在 Node runtime server component 里（防止 Edge crypto 不兼容）。

### Stats
- 3 commits：`(1) feat(admin-portal): Phase C.4 admin auth` + `(2) feat(agent-os): acceptance_bt_editor` + `(3) docs(2.0): Phase C GA`（本文）
- 0 pre-existing tests broken
- admin-portal tests：50 → 64（+14）
- agent-os acceptance steps：6 (emotion) → 13 (emotion + bt_editor)
- Spec & Plan: `docs/superpowers/plans/synthetic-jumping-jellyfish.md` §Phase C.4
- **3-phase backlog 关闭**（OCEAN→emotion + Saga viz + BT editor）

---

# 2.0.5-stage3-phase-b-saga-viz (2026-10-06)

## [Phase B] Saga DSL React Flow 只读可视化 — 2026-10-06

Saga DSL YAML → React Flow 流程图只读可视化：admin-portal `/saga-viz` 路由 +
dropdown 选 saga + React Flow 渲染 + dagre 自动布局 + 元数据面板。JS-yaml 解析
YAML；自定义 SagaStepNode 按 type 着色（forward 绿 / compensation 橙 /
start-end 靛蓝）。只读，无编辑器。

### 关键变更
- 新增 admin-portal `/saga-viz` 路由（dropdown + graph + 元数据面板）
- `saga_loader.ts` — `parseSagaToGraph()` + 4 types（js-yaml 驱动）
- `saga_layout.ts` — `@dagrejs/dagre` TB 自动布局
- `SagaStepNode.tsx` — custom node（type 配色 + handles 上下对）
- `SagaGraph.tsx` — React Flow wrapper（节点应用 layout 坐标）
- `SagaStepNode` 含 `data-testid="saga-node-${label}"`（E2E 可断言）
- API routes：GET `/api/sagas`（list + sort）+ GET `/api/sagas/[name]`（detail + safeName）
- 主页 `/` 加 nav link 到 `/saga-viz`
- 1 Playwright E2E smoke（mock API + 验证 8 节点 + 元数据）
- 3 新 deps：`js-yaml@^4.1.0`、`@dagrejs/dagre@^1.1.0`、`@playwright/test@^1.49.0`
- 3 测试文件：`saga_loader.test.ts` (8)、`saga_layout.test.ts` (4)、`saga-viz.spec.ts` (1)

### 影响
- **性能**: layout 计算纯内存 O(n+m)（n=nodes, m=edges），welcome_3npc 8 节点 <5ms；渲染受 React Flow 虚拟化保护
- **向后兼容**: 仅新增 `/saga-viz` 路由 + nav；YAML 格式不变；0 impact on 现有 saga-orchestrator 加载流程
- **零回归**: pre-existing 12 vitest tests + 新增 1 E2E 全部 PASS

### 已知限制
- 无 auth（仅 dev 工具；YAGNI 推迟 admin auth gate）
- YAGNI 编辑器（spec §7 阶段 2 推迟）：当前只读，编辑出图/DSL lint 留待 backlog
- E2E 测试使用 mock API（不依赖文件系统）；真实 saga-scripts 渲染留待后续 acceptance binary

### Stats
- 3 commits ahead of pre-Phase-B origin: `a330953` + `6b97dfd` + B.3
- 0 pre-existing tests broken
- Loader: 8 单测；Layout: 4 单测；E2E: 1 — 合计 13 测试
- New routes: `GET /saga-viz`（page）+ `GET /api/sagas` + `GET /api/sagas/[name]`
- Spec & Plan: `docs/superpowers/plans/synthetic-jumping-jellyfish.md` §Phase B

---

# 2.0.4-stage3-phase-a-ocean-bias (2026-10-06)

## [Phase A] OCEAN → emotion 偏好 — 2026-10-06

OCEAN 5 维度人格 → 8 类 emotion 偏好概率派生（线性加性模型），注入
【人格基线情绪】段到 system prompt。改 NPC OCEAN 配置后 baseline 分布
会按预期偏移。

### 关键变更
- 新增 `agent_os/ocean/` 包（bias.py + coefficients.py + __init__.py）
- `NpcTemplate.baseline_emotion_distribution` 字段 + 启动期派生
- `get_npc_stream_prompt` 加 `baseline_distribution` kwarg + `_render_baseline_section()`
- `dispatcher.say_stream` 拉 baseline (best-effort)
- 新 env var `OCEAN_BIAS_ENABLED` (kill switch, default true)
- 6 子任务 A.1-A.6，47 个测试通过
- acceptance binary `acceptance_emotion_v1.py` 6/6 PASS

### 影响
- **性能**: dispatch 路径增加 1 次同步 dict lookup（O(1)）+ 1 次字符串拼接，<1ms
- **向后兼容**: `OCEAN_BIAS_ENABLED=false` 时 prompt byte-identical pre-A
- **零回归**: pre-existing 2 failures (`tests/llm/test_claude_real.py` + `test_integration.py`) 保留

### 已知限制
- 系数（OCEAN_BIAS_BASE + COEFFICIENTS）经验值；后续可用 stage 2 离线评估集做自动化调优
- 5 维 OCEAN YAML 已存在 6 NPC；YAGNI 推迟 logit 采样调制（spec §10）

### Stats
- 7 commits ahead of pre-A origin: `722960f` + `53cee2c` + `51d9aa9` + `ca99519` + `4f190bf` + `287397c`
- 0 pre-existing tests broken (pre-existing 2 failures 保留)
- Coefficients: 14；NpcTemplate baseline: 4；Settings ocean: 5；Prompts ocean: 9；Dispatcher ocean: 4；Acceptance: 6 — 合计 42 单测 + 5 acceptance binary 步骤
- Acceptance binary: `apps/agent-os/scripts/acceptance_emotion_v1.py`

---

# 2.0.3-stage3-b2-emotion-persistence (2026-10-06)

## [B2] Emotion 持久化 — 2026-10-06

### Highlights
- **Emotion 持久化 + 衰减聚合**：阶段 2 的 8 类 emotion 标签现在写入 PG；agent-os 下一回合拉历史 → Python 端指数衰减 → 注入 `【最近情绪氛围】` 进 system prompt
- 玩家↔NPC 对话有**情绪惯性**：per-player×NPC（τ=2h 快衰）+ per-NPC-global（τ=24h 慢衰）
- 5 新 env vars 含 1 个 kill switch；新 acceptance binary 6 步 E2E

### Added

- **DB Migration** `db/migrations/pg-schema-2.0-emotion.sql` — `memory_player_session` 表新增 `emotion TEXT` 列 + 复合索引 `(npc_id, player_id, created_at DESC, emotion)`
- **`agent_os.emotion.aggregate`** — 纯数学模块，`exp_decay_weight(age_seconds, tau_seconds)` + `aggregate_distribution(rows, tau, now)`（12 单测覆盖空 / 单行 / 全部同时间 / 跨 τ 边界）
- **`agent_os.emotion.repository`** — asyncpg 包装，两个查询：
    - `fetch_player_distribution(npc_id, player_id, limit, now)`（per-player×NPC 近期）
    - `fetch_global_distribution(npc_id, limit, now)`（per-NPC-global）
  （5 单测覆盖 timeout / empty / happy path）
- **`agent_os.emotion.settings`** — env-driven dataclass，5 个 env vars，校验 fail-loud（6 单测）
- **`agent_os.memory.writer.write_emotions`** — best-effort 批量写 emotion 行（3 单测）
- **`agent_os.llm.prompts.get_npc_stream_prompt`** — 新增 `recent_distribution` + `global_distribution` kwargs（Optional[Dict[str, float]]），未传 / 关闭时不渲染（向后兼容）；传入时渲染 `【最近情绪氛围】` 段（5 单测）
- **App wiring** `agent_os.app` lifespan — 启动期实例化 `EmotionRepository`（共享 pool），注入到 dispatcher
- **Dispatcher** `agent_os.dispatcher.say_stream` — 注入开启时（`EMOTION_INJECT_ENABLED=true`）在 prompt build 前拉分布；fetch 失败 fallback 不注入（不抛）；mark_done 后调用 `write_emotions` 持久化（best-effort）
- **Acceptance** `scripts/acceptance_emotion_v0.py` — 6 步 E2E binary（mocked unit tests，8 测试）：schema check / aggregate pure math / repository mock / settings env parse / writer mock / dispatcher hook

### Env vars (5 new)

| Env var | Default | Purpose |
|---------|---------|---------|
| `EMOTION_TAU_PLAYER_SECONDS` | 7200 (2h) | Per-player×NPC 衰减 τ |
| `EMOTION_TAU_GLOBAL_SECONDS` | 86400 (24h) | Per-NPC-global 衰减 τ |
| `EMOTION_PLAYER_LIMIT` | 50 | Per-player 扫描行数上限 |
| `EMOTION_GLOBAL_LIMIT` | 100 | 全局扫描行数上限 |
| `EMOTION_INJECT_ENABLED` | true | Kill switch（false = 跳过聚合 + 注入） |

### Fixed

- **asyncpg 缺失**：B2-T03 follow-up 把 `asyncpg>=0.30` 加到 `agent-os` 的 `pyproject.toml` 依赖（`fb8ca64` commit）

### Performance

- **Best-effort 持久化**：emotion 写入不阻塞 LLM 流；fetch 失败 fallback 为 no-injection 而非 raise
- **指数衰减数学**：O(n) over n=扫描行数（n≤100），每次 stream 帧调用一次，两次 SQL 查询（player + global）已加 timeout
- **共享 pool**：`EmotionRepository` 复用 lifespan 启动期建的 asyncpg pool（不开新连接）

### 范围外（YAGNI）

- OCEAN personality → emotion 偏好概率（backlog，独立 sub-spec）
- emotion 历史写入 Milvus 向量库（当前仅 PG 行级）
- emotion 跨城联邦转发（emotion 仍仅本城）
- emotion 时间窗 UI 可视化（admin / player 都看不到）

### Stats

- 16 commits ahead of pre-B2 origin
- 0 pre-existing tests broken
- Aggregate: 12 单测；Repository: 5；Settings: 6；Writer: 3；Prompts: 5；Wiring: 9；Dispatcher hook: 6；Acceptance: 8 — 合计 54 个 B2 测试
- Acceptance binary: `apps/agent-os/scripts/acceptance_emotion_v0.py`

### Spec & Plan

- spec: `docs/superpowers/specs/2026-10-06-2.0-emotion-persistence-design.md`
- plan: `docs/superpowers/plans/2026-10-06-2.0-emotion-persistence.md`

---

# 2.0.2-stage3-b1-cross-city-stream (2026-10-06)

## [B1] 跨城 NPC 流式 — 2026-10-06

### Highlights
- **跨城联邦流式 (cross-city NPC streaming)**: A 城玩家与 B 城 NPC 流式对话，端到端 SSE
- 新 gRPC RPC `SayStreamForward` (a2a.proto)
- 新 HTTP endpoints（a2a-gateway + api-gateway）
- 新错误码 R_016 / R_017

### Tasks (11 total)
- T01 proto: SayStreamForward + SayRequestInit + SayBeat
- T02 MirrorSessionStore: 跨城会话缓冲
- T03 gRPC server (B-city): SayStreamForward 实现骨架
- T04 SSE entry (a2a-gateway): POST /v1/federation/say_stream
- T05 errors: R_016 / R_017
- T06 Forwarder (A-city): Redis sub → mirror → SSE pump
- T07 api-gateway relay: POST /v1/world/cross_city_say_stream
- T08 replay endpoint: GET /v1/federation/sessions/{sid}/buffer
- T09 acceptance_2_2: 5-step E2E binary
- T10 docs (this)
- T11 integration verification + cleanup

### New Endpoints
| Method | Path | Service | Purpose |
|--------|------|---------|---------|
| POST | `/v1/federation/say_stream` | a2a-gateway | SSE stream from B-city NPC |
| GET | `/v1/federation/sessions/{sid}/buffer` | a2a-gateway | replay beats after disconnect |
| POST | `/v1/world/cross_city_say_stream` | api-gateway | SSE relay to a2a-gateway |

### New Error Codes
- **R_016 CrossCityStreamFail** (HTTP 502) — 跨城流式 RPC 失败
- **R_017 CrossCityStreamTimeout** (HTTP 504) — 跨城流式 RPC 超时

### Stats
- 16 commits ahead of pre-B1 origin
- 0 pre-existing tests broken
- Acceptance binary: `cmd/acceptance_2_2/`

### Spec & Plan
- spec: `docs/superpowers/specs/2026-10-06-2.0-stage3-cross-city-stream-design.md`
- plan: `docs/superpowers/plans/2026-10-06-2.0-stage3-cross-city-stream.md`

---

# 2.0.1-stage2-stream-emotion (2026-10-05)

## 新增功能

### agent-os 流式管线（Sub-spec A）
- `dispatcher.say_stream()` — LiteLLM 真流式 → 句子节拍事件流（token → sentence → Redis）
- `SentenceSplitter` — XML `<emotion=X>` tag 流式切句（generator state-safe）
- `EmotionValidator` — 8 类 emotion 验证 + 异常降级 neutral
- `Publisher` — Redis 频道 `aicity:npc:say_stream` + 100 条 buffer 重试 + 幂等 drain
- `SessionStore` — 60min TTL 内存 session + 重连补帧（frame_no 序号）
- `EndMarker` — `<end>` tag / 3s 超时 → done 包 + 幂等关闭
- `LiteLLMProvider.stream()` — 真流式 token-by-token（防御性 guard）
- 5 NPC 流式 prompt 模板（wang_boss / grace_healer / snack_owner / book_keeper / dance_leader，emotion tag 强制）
- 错误码 R_011~R_015（LLM_STREAM_FAIL / TIMEOUT / EMOTION_PARSE / REDIS_PUB / SESSION_NOT_FOUND）

### ws-gateway 多频道
- 默认频道列表新增 `aicity:npc:say_stream`
- subscriber wiring + protocol 常量
- a2a-gateway `streamcheck` 包（Login + SubscribeBeat 烟囱）

### web 端逐句渲染
- NPCDialog 逐句渲染（sentence-by-sentence typewriter）
- 8 类 emotion emoji overlay（joy / sad / angry / fear / surprise / disgust / neutral / shy）
- CSS fade-in 动画

### 测试与验证
- agent-os 集成测试 — 5 NPC 流式管道 + emotion 降级
- agent-os 单测 — EndMarker 幂等、SentenceSplitter flush、whitespace/int/list 边界
- LLM prompt personality/context test + npc_context guard
- acceptance_2_1 binary — 5 步 E2E（登录 → 5 NPC 流式 → emotion 验证）
- Playwright E2E 5 case（5 NPC 各 1 流式 + emotion overlay）
- 离线评估 200 条 + emotion classifier 脚本（emotion 分类准确率 40% → 85%）
- a2a-gateway acceptance_2_1 Dockerfile 镜像集成

## 升级

- agent-os `dispatcher.__init__` 新增 `publisher` 参数
- ws-gateway 默认频道列表加 `aicity:npc:say_stream`
- docker-compose agent-os 加 `REDIS_CHANNEL_SAY_STREAM` env
- e2e 跑真 Haiku（`--run-real-llm` gating + 真实 timeout wrap）

## 破坏性变更

无（向后兼容 stage1 的 `dispatcher.say()` 一次性接口）

## 范围外（YAGNI）

- BT 编辑器 v2（sub-spec B 独立）
- Saga DSL 可视化（sub-spec C 独立）
- 跨城 NPC 流式转发（stage3 延伸）

## 10 个里程碑 commits

| Task | SHA | 说明 |
|---|---|---|
| T01 spec | `d219582` | spec(2.0-stage2): LLM 流式 + emotion 表情（sub-spec A） |
| T01 spec | `53c6155` | spec(2.0-stage2): self-review fixes — buffer 上限下沉到 plan |
| T01 plan | `88fd443` | plan(2.0-stage2): 31 tasks 实施 plan |
| T02 splitter | `7e9b452` + `6265bfc` | SentenceSplitter + generator state-safety |
| T03 validator | `be206f7` | EmotionValidator — 8 类验证 |
| T03 session | `728d25d` | SessionStore — 60min TTL + 重连补帧 |
| T04 errors | `016db49` | R_011~R_015 完整定义 |
| T04 endmarker | `78b192d` + `0b79cb4` | EndMarker + 幂等测试 |
| T05 publisher | `4ec64f2` + `bb624c5` | Publisher + buffer off-by-one |
| T06 dispatcher | `8145553` | dispatcher.say_stream() |
| T07 LLM | `eadf2f5` + `badf059` | LiteLLMProvider.stream() + 防御性 guard |
| T08 prompts | `1259f8b` + `0574a47` | 5 NPC 流式 prompt + personality test |
| T09-R015 | `1d528dd` | drop redundant R015SessionNotFound |
| T10 测试 | `0db7476` | whitespace/int/list 边界 |
| T11 ws | `c0d039b` + `9871375` | ws-gateway 多频道 + subscriber wiring |
| T11 env | `2229d38` | REDIS_CHANNEL_NPC_SAY_STREAM env |
| T13 e2e | `83eb908` + `1c695b5` | say_stream e2e + 真实 Haiku |
| T14-T17 accept | `a57f429` + `dda7279` + `b0645ef` | acceptance_2_1 step 1-5 + Dockerfile |
| T14 streamcheck | `0555ad8` | a2a-gateway streamcheck 包 |
| T18 web | `bac4c96` | NPCDialog sentence-by-sentence 渲染 |
| T19 测试 | `fffd1fb` | NPCDialog 单测 |
| T20 E2E | `30a8091` | Playwright 5 NPC 流式 + emotion E2E |
| T22 data | `edc0384` | 200 条 emotion 调优数据 |
| T23 eval | `63b60da` | emotion classifier offline eval (200 cases) |
| T24 tune | `348e53f` | prompt tune — emotion 准确率 40% → 85% |
| T25 ROADMAP | `fe92a16` | docs(2.0): ROADMAP 加上 stage 2 + stage 3 |
| T26 CHANGELOG | (本 commit) | docs(CHANGELOG-2.0): 2.0.1-stage2-stream-emotion 条目 |

## 验证

- acceptance_2_1 binary：5/5 步骤通过（5 NPC 流式 + emotion）
- 13 容器 healthy 验证
- Playwright E2E：5 NPC 流式 + 8 类 emotion overlay
- 离线评估：emotion 分类准确率 85%（≥70% 阈值）
- 1min 录屏脚本（3 镜头 + 应急方案）

---

# 2.0.0 (2026-10-04)

## 新增功能

### LLM 接入（Phase 2）
- Claude Sonnet 4.6 via LiteLLM 抽象
- 5 NPC prompt 模板（wang_boss / grace_healer / snack_owner / book_keeper / dance_leader）
- chat_turns=6 强制收尾 + token cap 双重成本保险
- 短路词（问候/告别不走 LLM）
- 错误码 R_001~R_010（LLM/Milvus/Saga/跨城/输入/上游）
- 成本监控（$3 input / $15 output per 1M，月预算 $100）

### 记忆系统（Phase 3）
- memory-service: FastAPI + Milvus + bge-large-zh-v1.5 (1024-dim)
- /v1/recall (top-K=5) + /v1/write API
- LRU 缓存 + PG fallback（Milvus 不可达时降级）
- 离线评估脚本 + 1000 条标注集（Recall@5 ≥ 70% 阈值）

### Saga 引擎（Phase 4）
- saga-orchestrator (Go): StateMachine + 5 状态 + 内存 Store
- saga-worker (Python): Kafka consumer + idempotent
- /v1/saga/trigger 端点 + 补偿事务（force_fail 注入）
- Prometheus 指标（saga_compensation_total / saga_trigger_total）
- welcome_3npc.yaml 协作剧本（3 NPC × 跨城）

### 跨城联邦（Phase 5）
- a2a-gateway 路由表（10 条 YAML 路由，5 NPC × 2 城）
- mTLS（自签 CA + 6 服务证书）
- 跨城 gRPC client（rustls on world-engine / grpc-go on a2a-gateway）
- world-engine 心跳上报（每 30s）
- 跨城 HTTP 转发 + Kafka fanout + ws 推送
- 证书自动轮换脚本（到期前 7 天）
- 7 天稳定 cron + 跨城连通测

### 桶 2 P1（Phase 6）
- 5 NPC 全 enabled（grace_healer / snack_owner / book_keeper / dance_leader 新增 yaml）
- avatar_url 字段 + web 端 SVG emoji fallback

### 集成（Phase 7）
- acceptance_2_0 binary（5 步 E2E：登录 → LLM → 记忆 → Saga → 跨城）
- 4 退出码分类
- 跨轨道联调 smoke + Playwright 2 case

## 升级

- 容器数 8 → 13（postgres × 2、redis × 2、world-engine × 2、api-gateway × 2、ws-gateway × 2、web × 2、agent-os × 2、a2a-gateway、memory-service、saga-orchestrator、saga-worker、Milvus、Kafka）
- 错误码前缀 R_（agent-os） + F_（a2a-gateway 已有）
- NPC ID 格式：`npc_<city>_<slug>`（如 `npc_a_wang_boss`）

## 破坏性变更

- 1.0 玩家数据不兼容（新增 `npc_id` 全局格式字段）
- world-engine gRPC 端口变更（50051 → 50061）
- 路由表 schema 替换 1.0 静态注册

## 桶 1 已知限制

- 不接 LLM 流式输出（仅一次性 complete）
- 记忆召回阈值仅 70%
- 跨城延迟未调优（P50 ~200ms 目标）
- 5 NPC 仅启用中文对话

## 桶 2 显式不做（2.0 范围外）

- LLM 流式 + emotion 表情动画（2.0 阶段 2）
- BT 编辑器 + Saga DSL（2.0 阶段 3）
- Push 通知 + 离线模式（2.1）
- 客户端预测 + 经济系统（2.2）

## 验证

- acceptance_2_0 binary：5/5 步骤通过（~30s）
- 13 容器 healthy 验证
- Playwright E2E：LLM-NPC 上下文 + 跨城路由
