# a2a-gateway

> **职责**：A2A v0.1 联邦协议 + Agent Card + Envelope + openClaw/workbuddy 真 HTTP 适配器
>
> **关键文档**：[docs/06-A2A协议.md](../../docs/06-A2A协议.md) 全文
>
> **复盘**：[docs/SPRINT-5.md](../../docs/SPRINT-5.md) · [docs/SPRINT-6.md](../../docs/SPRINT-6.md) · [docs/SPRINT-7.md](../../docs/SPRINT-7.md) · [docs/SPRINT-7+.md](../../docs/SPRINT-7+.md)

## 端口

| 类型 | 端口 | 环境变量 | 说明 |
|---|---|---|---|
| gRPC | `50061` | `A2A_GRPC_ADDR`（默认 `127.0.0.1:50061`） | Sprint 5+；A2AGateway 5 RPC（Sprint 7 + FetchInbox） |
| HTTP | `8083` | `A2A_HTTP_ADDR`（默认 `127.0.0.1:8083`） | Sprint 6+；in-process 双协议网关 |

## 协议能力（Sprint 5 MVP）

- Agent Card 注册（`RegisterCard`） + 联邦发现（`Discover`）
- Envelope 投递（`SendMessage`） + 双向流（`Stream`，EchoAdapter）
- A2A 错误码 F_001/F_002/F_003/F_004/F_005（§20.10）
- 内存注册表（重启即清）

## 协议能力（Sprint 5.5）

- ed25519 签名校验（§20.11-13）：`auth["ed25519"]` 公钥注册 + 重放窗口（默认 5min）
- Adapter 框架（§20.14）：`Dispatcher` 选路 + `EchoAdapter` 兜底
- 错误码增量：F_006（pubkey）/ F_007（signature）/ F_008（ts_ms）/ F_009（provider 路由）

## 协议能力（Sprint 6）

- **HTTP gateway**（同进程双协议）：4 端点 + Bearer 鉴权（可选）+ trace_id 透传
- **真 HTTP outbound Adapter**：`openclaw` / `workbuddy` → `POST {recipient.URL}/inbox`
  - 共享 `*http.Client{Timeout: 5s}` + `Transport.MaxIdleConnsPerHost=4`
  - 200 + JSON → 解 reply；204 → fire-and-forget；4xx/5xx/超时 → F_010
- **错误码增量**：F_010（upstream 不可达 / 4xx 5xx / reply 解码失败）

## 协议能力（Sprint 7）

- **PG 持久化**：`a2a_agent_card`（mirror AgentCard）+ `a2a_inbox`（store-and-forward）
  - 替换 Sprint 5 内存 `Registry`（重启即清）；用 `pgxpool` 直查 DB
  - 同进程双协议 + PG 双写：gRPC/HTTP 共享 `*a2asrv.Service` 单例 + 单一 `CardStore`/`InboxStore`
- **`FetchInbox` RPC**（gRPC + HTTP）：拉取 `a2a_inbox` 未读消息；支持 `cursor` 增量 + `mark_read`
- **`InboxAdapter`** 替换 `EchoAdapter`：写 inbox 后返 `(nil, nil)`（fire-and-forget）
- **`HTTPAdapter` inbox fallback**：POST 失败 → 写 inbox → 返 success（store-and-forward）
- **错误码增量**：
  - F_011 inbox 写失败 → 500
  - F_012 inbox 读失败 → 500
  - F_013 agent_id 未注册（FetchInbox） → 404
  - F_014 FetchInbox limit 非法 → 400
- **F_001-F_014 → HTTP status 映射**：

  | F-code | HTTP | 含义 |
  |---|---|---|
  | F_001 | 400 | agent_id/name 缺失 |
  | F_003 | 400 | capability 为空 |
  | F_004 | 404 | 收件方未注册 |
  | F_005 | 401 | 发件方未注册 |
  | F_006 | 400 | pubkey 解析失败 |
  | F_007 | 401 | signature 失败 |
  | F_008 | 401 | ts_ms 出窗 |
  | F_009 | 400 | provider 路由失败 |
  | F_010 | 502 | upstream 不可达 |
  | F_011 | 500 | inbox 写失败 |
  | F_012 | 500 | inbox 读失败 |
  | F_013 | 404 | agent 未注册（FetchInbox） |
  | F_014 | 400 | FetchInbox limit 非法 |

## HTTP 路由

| Method | Path | Handler | 鉴权 |
|---|---|---|---|
| GET  | `/v1/healthz` | Healthz | 公开 |
| POST | `/v1/cards` | RegisterCard | Bearer* |
| GET  | `/v1/discover` | Discover | Bearer* |
| POST | `/v1/messages` | SendMessage | Bearer* |
| GET  | `/v1/inbox/:agent_id` | FetchInbox | Bearer* |

*Bearer 仅当 `A2A_HTTP_API_KEY` 非空时启用；`/v1/healthz` 永远公开。

`GET /v1/inbox/:agent_id` 查询参数：
- `limit`：默认 50，最大 500（→ F_014）
- `cursor`：上一批的 `next_cursor`（RFC3339Nano）
- `mark_read`：true = 拉取即标已读

## 启动

```bash
# 必备：PG 在线（连接 DATABASE_URL 默认 aicity@aicity_dev/localhost:5432/aicity）
docker run -d --name aicity-pg -e POSTGRES_USER=aicity -e POSTGRES_PASSWORD=aicity_dev \
  -e POSTGRES_DB=aicity -p 5432:5432 postgres:15-alpine
docker exec -i aicity-pg psql -U aicity -d aicity < packages/proto/pg-schema.sql

# 双协议 server（默认 gRPC :50061 + HTTP :8083 + PG）
A2A_GRPC_ADDR=127.0.0.1:50061 \
A2A_HTTP_ADDR=127.0.0.1:8083 \
DATABASE_URL=postgresql://aicity:aicity_dev@127.0.0.1:5432/aicity \
  ./bin/a2a-gateway.exe

# 启用 Bearer 鉴权（dev 留空）
A2A_HTTP_API_KEY=dev-secret ./bin/a2a-gateway.exe
```

## 端到端验证

```bash
# 1) gRPC smoke（12 项 Sprint 5+5.5）
A2A_GRPC_ADDR=127.0.0.1:50061 ./bin/a2a_smoke.exe

# 2) HTTP smoke（8 项 Sprint 6）
A2A_HTTP_ADDR=http://127.0.0.1:8083 ./bin/http_smoke.exe

# 3) Inbox smoke（6 项 Sprint 7）
A2A_HTTP_ADDR=http://127.0.0.1:8083 ./bin/inbox_smoke.exe

# 4) curl 手动验证
curl -s http://127.0.0.1:8083/v1/healthz
curl -s -X POST http://127.0.0.1:8083/v1/cards \
  -H 'Content-Type: application/json' \
  -d '{"agent_id":"alice","name":"Alice","provider":"aicity","capabilities":["chat"]}'
curl -s 'http://127.0.0.1:8083/v1/discover?capability=chat'
curl -s -X POST http://127.0.0.1:8083/v1/messages \
  -H 'Content-Type: application/json' \
  -d '{"message_id":"m1","from_agent_id":"alice","to_agent_id":"bob","type":"request","payload":"aGk=","ts_ms":0}'
curl -s 'http://127.0.0.1:8083/v1/inbox/bob?limit=10&mark_read=true'
```

## 测试

```bash
go test ./...                                          # 全部单元 + bufconn + httptest 集成测试
go test -v -run TestService_ ./internal/a2asrv        # 仅 Service gRPC 测试
go test -v -run TestRouter_ ./internal/httpgw          # 仅 HTTP gateway 测试
go test -v -run TestHTTPAdapter_ ./internal/a2asrv     # 仅 HTTPAdapter 测试
go test -v -run TestFCodeToHTTP_ ./internal/httpgw     # 仅 errmap 错误码映射

# PG 集成测（env-gated，未设 → t.Skip）
A2A_TEST_DATABASE_URL=postgresql://aicity:aicity_dev@127.0.0.1:5432/aicity_test \
  go test -v -run "TestCardStore_PG|TestInboxStore_PG" ./internal/a2asrv
```

## 环境变量汇总

| 变量 | 默认 | 说明 |
|---|---|---|
| `A2A_GRPC_ADDR` | `127.0.0.1:50061` | gRPC 监听地址 |
| `A2A_HTTP_ADDR` | `127.0.0.1:8083` | HTTP 监听地址 |
| `A2A_HTTP_API_KEY` | 空（关鉴权） | HTTP Bearer token；非空时强制校验 |
| `A2A_REPLAY_WINDOW_SEC` | `300` | ed25519 重放窗口秒数 |
| `DATABASE_URL` | `postgresql://aicity:aicity_dev@localhost:5432/aicity` | PG 连接串（Sprint 7） |
| `A2A_TEST_DATABASE_URL` | 空（跳过 PG 测） | PG 集成测 env-gate；本地 dev 设上 |
| `A2A_INBOX_TTL_HOURS` | `168` | a2a_inbox 行 TTL 小时数；0 = 用 PG DEFAULT 兜底（Sprint 7+） |
| `A2A_INBOX_CLEANUP_INTERVAL_SEC` | `300` | cleanup cron 间隔秒数；0 = 禁用（Sprint 7+） |

## 不在 Sprint 7 范围

- ACL + 跨城邦路由 → Sprint 7+
- AgentCard 自签 / CA → Sprint 8+
- HTTP 流式镜像（SSE / WebSocket）→ Sprint 8+（确认需求后）
- canonical form 单点化到 `packages/sdk-go`（Python/TS SDK 对齐）→ Sprint 7+

## Sprint 7+ "硬化"

两块累积技术债的扫尾：

### A. Canonical SDK 单点化

消除 server / SDK / smoke 三份 canonical 实现漂移风险：

- **唯一实现**：[`packages/sdk-go/canonical.go`](../../packages/sdk-go/canonical.go)::`CanonicalBytes(s Signable)`
- **server 端薄适配器**：`apps/a2a-gateway/internal/a2asrv/verifier.go::canonicalBytes(m)` → proto → `aicity.Signable` → `aicity.CanonicalBytes`
- **跨实现护栏**：`verifier_test.go::TestVerifier_CanonicalBytes_MatchesSDK` byte-equal 比对 + 真实验签回路；任何漂移立刻 fail
- **smoke 同源**：`cmd/a2a_smoke/main.go::signCanonical` 改用 `aicity.SignMessage`，消除第三份

### B. `a2a_inbox` TTL + cron 清理

防止生产跑 1 个月 inbox 表堆积：

- **新列 `expires_at TIMESTAMPTZ NOT NULL DEFAULT (NOW() + '7 days')`**
  （`pg-schema.sql` ALTER ADD COLUMN IF NOT EXISTS；旧行 metadata-only 默认 7d）
- **B-tree 索引 `idx_a2a_inbox_expires`**（无 partial predicate，避免 volatile 函数被 PG 拒绝）
- **`InboxStore.Cleanup(ctx)`** 删除 `expires_at < NOW()` 的行
- **后台 cron `StartInboxCleanup(ctx, store, interval)`**：
  - 独立 long-lived ctx 模式（与 api-gateway subscriber 同坑）
  - Cleanup SQL 用 30s 独立 timeout，让最后一次能跑完
  - interval=0 禁用；store=nil 跳过；仅 log 失败（fire-and-forget）
- **`A2A_INBOX_TTL_HOURS`**（默认 168 = 7d，0 = 用 PG DEFAULT 兜底）
- **`A2A_INBOX_CLEANUP_INTERVAL_SEC`**（默认 300 = 5min，0 = 禁用）

### Schema 迁移（幂等）

```bash
docker exec -i aicity-pg psql -U aicity -d aicity < packages/proto/pg-schema.sql
docker exec -i aicity-pg psql -U aicity -d aicity -c '\d a2a_inbox' | grep expires_at
docker exec -i aicity-pg psql -U aicity -d aicity -c 'SELECT version FROM schema_version ORDER BY version;'
# 期望：2.3.0 / 2.4.0 / 2.5.0
```

`ALTER TABLE ADD COLUMN ... DEFAULT (expr)` 在 PG 11+ 是 metadata-only，
不重写表；旧行自动回落到默认 7d。

### 测试覆盖

| 模块 | 用例 |
|---|---|
| `packages/sdk-go/canonical_test.go` | 7 用例（EmptyAllBlank / Deterministic / FieldOrder / PayloadRawStd / PaddingBoundary / StableAcrossReorder / SignMessage_Roundtrip）|
| `verifier_test.go` | +1 用例 `TestVerifier_CanonicalBytes_MatchesSDK`（server byte-equal SDK + 真实验签回路）|
| `inboxstore_pg_test.go` | +2 用例 `TestInboxStore_PG_TTL_Expires` + `TestInboxStore_PG_DefaultTTL`（env-gated）|
| 现有 74 测 | 全部继续 PASS（签名 byte-equal + Append/Fetch 行为不变）|
