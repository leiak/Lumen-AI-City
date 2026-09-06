# Sprint 7+ 复盘

> 范围：**a2a-gateway — Canonical SDK 单点化 + a2a_inbox TTL/cron**
>
> 完成时间：2026-09-06
>
> 提交：
> - `feat(sprint7+): canonical SDK 单点化 + inbox TTL/cron` (`53e6609`)
> - 后续完善：`docs/06-A2A-canonical.md §五` 更新 + `cron_test.go` 加 no-op 路径

## 一、本次交付

两块累积技术债的扫尾：

### A. Canonical SDK 单点化

消除 server / SDK / smoke 三份 canonical 实现漂移风险：

| 项 | 说明 |
|---|---|
| 唯一实现 | `packages/sdk-go/canonical.go::CanonicalBytes(s Signable) []byte` —— 整个 aicity 联邦唯一 |
| server 薄适配器 | `apps/a2a-gateway/internal/a2asrv/verifier.go::canonicalBytes(m)` → proto → `aicity.Signable` → `aicity.CanonicalBytes` |
| 跨实现护栏 | `verifier_test.go::TestVerifier_CanonicalBytes_MatchesSDK`：byte-equal 比对 + 真实验签回路；任何漂移立刻 fail |
| SDK 签名 | `packages/sdk-go/signing.go::SignMessage` 改调 `CanonicalBytes`（删 `encoding/json` import） |
| smoke 统一源 | `cmd/a2a_smoke/main.go::signCanonical` 改用 `aicity.SignMessage`（消除第三份） |
| go.mod | `require github.com/aicity/sdk-go v0.0.0` + `replace ... => ../../packages/sdk-go`（消除 v0.0.0 远程 fetch） |

### B. a2a_inbox TTL + cron

防生产跑 1 个月表堆积：

| 项 | 说明 |
|---|---|
| 新列 | `a2a_inbox.expires_at TIMESTAMPTZ NOT NULL DEFAULT (NOW() + '7 days')`（PG 11+ metadata-only ADD COLUMN，旧行自动 fallback） |
| 索引 | `idx_a2a_inbox_expires ON a2a_inbox(expires_at)` 普通 B-tree（无 partial predicate —— PG 拒绝含 volatile 函数） |
| Cleanup | `InboxStore.Cleanup(ctx)` 删除 `expires_at < NOW()` 的行；返 (int64, error) |
| 后台 cron | `StartInboxCleanup(ctx, store, interval)`：独立 long-lived ctx + Cleanup SQL 用 30s 独立 timeout + interval=0 禁用 |
| 环境变量 | `A2A_INBOX_TTL_HOURS`（默认 168 = 7d，0 = 用 PG DEFAULT 兜底）+ `A2A_INBOX_CLEANUP_INTERVAL_SEC`（默认 300 = 5min，0 = 禁用）|
| 启动 banner | 含 `inbox_ttl=168h0m0s cleanup_interval=5m0s` 便于排障 |
| schema_version | bump 到 2.5.0（ON CONFLICT DO NOTHING 幂等）|

## 二、E2E 输出

```
$ go test ./packages/sdk-go/... ./apps/a2a-gateway/... -count=1
ok  	github.com/aicity/sdk-go	0.085s       (7 PASS)
ok  	github.com/aicity/a2a-gateway/internal/a2asrv	3.230s
ok  	github.com/aicity/a2a-gateway/internal/httpgw	0.110s
（总计 82 PASS / 0 FAIL；PG env 未设 → TestInboxStore_PG_TTL_Expires + DefaultTTL SKIP）

$ go vet ./packages/sdk-go/... ./apps/a2a-gateway/...
(0 errors)

$ go build -o bin/a2a-gateway.exe  ./apps/a2a-gateway/cmd
$ go build -o bin/a2a_smoke.exe     ./apps/a2a-gateway/cmd/a2a_smoke
$ go build -o bin/http_smoke.exe    ./apps/a2a-gateway/cmd/http_smoke
$ go build -o bin/inbox_smoke.exe   ./apps/a2a-gateway/cmd/inbox_smoke
(4 binaries built)

$ ./bin/a2a-gateway.exe
2026/09/06 16:39:51 a2a-gateway starting: grpc=127.0.0.1:50061 http=127.0.0.1:8083
  (replay_window=300s api_key=unset db=postgresql://aicity:***@localhost:5432/aicity
   inbox_ttl=168h0m0s cleanup_interval=5m0s)
```

> 注：本会话 Docker 未在线，PG 集成测 SKIP；a2a_smoke 12/12 + http_smoke 8/8 + inbox_smoke 6/6 端到端
> 验证需 Docker 在线时复跑。a2a_smoke 改用 SDK 后逻辑零改动，签名 byte-equal 由 TestVerifier_CanonicalBytes_MatchesSDK
> 保证。

## 三、关键决策

| # | 决策 | 理由 |
|---|---|---|
| 1 | `CanonicalBytes` 落点：`packages/sdk-go/canonical.go` | 单点化 = 物理上只有一份；server import SDK（go.work 已含两边） |
| 2 | server 端 adapter 形态：`verifier.go::canonicalBytes(m)` → `aicity.CanonicalBytes(aicity.Signable{...})` | 8 字段镜像，base64.RawStdEncoding 在 server 端做（proto 翻 Signable） |
| 3 | 跨实现护栏：`TestVerifier_CanonicalBytes_MatchesSDK` | server 输出 byte-equal SDK 输出 + 真实验签回路；任何漂移立刻 fail |
| 4 | `cmd/a2a_smoke::signCanonical` 本 sprint 一起 refactor | 消除第三份；smoke binary 已在 go.work 内，无新 import 摩擦 |
| 5 | TTL 落点：`a2a_inbox` 加 `expires_at` | 与 `queued_at`（audit 时间）解耦；旧行自动回落到默认 7d |
| 6 | TTL 默认 168h | 7 天足够绝大多数场景；env `A2A_INBOX_TTL_HOURS` 可调 |
| 7 | cleanup 间隔 300s | 5 分钟一次；与 api-gateway subscriber 一样的"独立 long-lived ctx"模式 |
| 8 | 普通 B-tree 索引 | PG 不允许 partial predicate 含 volatile 函数；plain B-tree 开销可忽略 |
| 9 | `NewInboxStore(pool)` 保留作 alias | 不破现有 5 个 PG 测试 + adapter 引用；新代码用 `NewInboxStoreWithTTL` |
| 10 | cron goroutine 模式：fire-and-forget + 独立 30s timeout | 与 api-gateway subscriber 同模式；最后一行 SQL 也能跑完 |
| 11 | `schema_version` bump 到 2.5.0 | 新列 + 新 cron + 新 env vars > patch；保留旧行 |
| 12 | proto 不动 | Sprint 7+ 不动 proto；`Message` 8 字段 + base64 编码无变化 |

## 四、变更清单

### 新增（5）

- `packages/sdk-go/canonical.go` —— 唯一 canonical 实现
- `packages/sdk-go/canonical_test.go` —— 7 用例
- `apps/a2a-gateway/internal/a2asrv/cron.go` —— `StartInboxCleanup` fire-and-forget
- `apps/a2a-gateway/internal/a2asrv/cron_test.go` —— 3 no-op 路径测试（plan 显式声明不测 ticker）
- `docs/SPRINT-7+.md` —— 本文件

### 修改（11）

- `packages/sdk-go/signing.go` —— `SignMessage` 改调 `CanonicalBytes`
- `packages/sdk-go/README.md` —— 加 "Canonical form" 章节
- `apps/a2a-gateway/go.mod` —— `require sdk-go v0.0.0` + `replace .../packages/sdk-go`
- `apps/a2a-gateway/internal/a2asrv/verifier.go` —— 删 local `canonicalEnvelope` struct；`canonicalBytes` 变薄适配器
- `apps/a2a-gateway/internal/a2asrv/verifier_test.go` —— +`TestVerifier_CanonicalBytes_MatchesSDK`（server byte-equal SDK + 真实验签回路）
- `apps/a2a-gateway/internal/a2asrv/inboxstore_pg.go` —— `InboxEntry.ExpiresAt` + `defaultTTL` + `Cleanup` + `NewInboxStoreWithTTL`；`NewInboxStore` 保留作 alias
- `apps/a2a-gateway/internal/a2asrv/inboxstore_pg_test.go` —— +2 用例：`TestInboxStore_PG_TTL_Expires` + `TestInboxStore_PG_DefaultTTL`（env-gated）
- `apps/a2a-gateway/cmd/main.go` —— env 读两值；`NewInboxStoreWithTTL`；launch `StartInboxCleanup`；banner 显示两值
- `apps/a2a-gateway/cmd/a2a_smoke/main.go` —— `signCanonical` 改用 `aicity.SignMessage`
- `packages/proto/pg-schema.sql` —— ALTER ADD COLUMN + CREATE INDEX + schema_version 2.5.0
- `apps/a2a-gateway/README.md` —— Sprint 7+ 章节 + 新 env vars 文档
- `docs/06-A2A-canonical.md §五` —— 变更协议更新为"唯一改动点 = packages/sdk-go/canonical.go::Signable"；新增§六"唯一实现"链接

## 五、关键陷阱

| # | 陷阱 | 缓解 |
|---|------|------|
| 1 | `packages/sdk-go` 模块加入 `a2a-gateway` 依赖（go.work 已含两边，无 replace 也能解析） | `go mod tidy` 后自动 |
| 2 | 抽 `CanonicalBytes` 不能改 Signable 字段顺序/tag/base64 编码 → 否则现存所有 agent 签名失效 | `TestVerifier_CanonicalBytes_MatchesSDK` byte-equal + `a2a_smoke 12/12` 真回路护栏 |
| 3 | cron 必须用 signal-cancel ctx（与 api-gateway subscriber 同坑） | 直接复用 `signal.NotifyContext` 的 ctx；cleanup SQL 用独立 `WithTimeout(30s)`，让最后一行能跑完 |
| 4 | `ALTER TABLE ADD COLUMN ... DEFAULT (expr)` 在 PG 11+ 是 metadata only，不重写表 | 旧行自动 fallback 到默认 7d；如担心大表可 `EXPLAIN` 验证 |
| 5 | partial index `WHERE expires_at < NOW()` PG 拒绝（volatile 函数）| 改用普通 B-tree（plan 已定）|
| 6 | `cmd/a2a_smoke` 改用 `aicity.SignMessage` 后仍是 in-process，不增 binary 依赖 | smoke binary 已在 go.work 内 |
| 7 | `NewInboxStore(pool)` 留作 alias → 5 个 PG 测试 + adapter 不动 | 双构造函数共存 |
| 8 | `ctx, stop := signal.NotifyContext(...)` 提到 serve 之前 → 必须确认 serve goroutines 不依赖旧顺序 | serve goroutines 用 `wg.Add`/`wg.Done`，独立运行；`<-ctx.Done()` 仍是关闭等待点 |
| 9 | schema_version `2.5.0` INSERT 用 `ON CONFLICT DO NOTHING`（与现有 `2.3.0`/`2.4.0` 模式一致） | — |
| 10 | **a2a-gateway → sdk-go v0.0.0**：go build 不报错（workspace mode），但 go test 卡远程 fetch（默认 proxy 不可达时 21s 超时） | 必须加 `replace github.com/aicity/sdk-go => ../../packages/sdk-go` |

## 六、回滚

- **代码**：`git revert 53e6609`（A+B 一起 revert）
- **Schema**：`ALTER TABLE a2a_inbox DROP COLUMN expires_at; DROP INDEX idx_a2a_inbox_expires;`（无其他代码引用）
- **Schema 版本行**：append-only，保留 `2.5.0` 历史

## 七、后续 Sprint 建议

按依赖顺序：

1. **Sprint 7+/8+**：ACL + 跨城邦路由（`cityFilter` 落地；agent ACL 策略；F_015/F_016）
2. **Sprint 8+**：AgentCard 自签 / CA 身份信任链（基于 Sprint 7+ 的 canonical + Sprint 8+ 的 ACL）
3. **Sprint 8+**：HTTP 流式镜像（SSE / WebSocket），镜像 gRPC `Stream` 已有能力
4. **远期**：Python/TS SDK 补 canonical 实现（按 docs/06-A2A-canonical.md §四 黄金向量对齐）