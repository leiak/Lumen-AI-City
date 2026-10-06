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
