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
