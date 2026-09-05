# Sprint 5.5 复盘

> 范围：**a2a-gateway — ed25519 验签 + 多 provider Adapter 框架**
>
> 完成时间：2026-09-05
>
> 提交：
> - `feat(sprint5.5): ed25519 verify + adapter framework (F_006/F_007/F_008/F_009)`

## 一、本次交付

### a2a-gateway

| 项 | 说明 |
|---|---|
| Verifier | `internal/a2asrv/verifier.go`：`Verifier{ReplayWindow}` + `ParsePublicKey` + `CanonicalBytes` + `Verify`；`SigError{Code,Reason}` |
| Registry F_006 | `auth["ed25519"]` 非空但公钥解析失败 → `accepted=false, errCode="F_006"` |
| Adapter | `internal/a2asrv/adapter.go`：`Adapter` interface + `Dispatcher`（map[provider]Adapter + fallback） |
| EchoAdapter | `internal/a2asrv/adapter_echo.go`：aicity 兜底；swap from/to + type="event" + 清空 signature |
| OpenClawStub / WorkbuddyStub | 占位 adapter（Sprint 6 替换为真 HTTP 转发） |
| Service | `NewService(reg, verifier, dispatcher)` 三参；SendMessage 走 verifier + dispatcher；Stream 失败关闭流（`codes.Unauthenticated`） |
| SDK | `packages/sdk-go/signing.go`：`ParsePublicKey` / `Signable` mirror struct / `SignMessage`；**不** import a2a proto |
| cmd | `cmd/main.go`：读 `A2A_REPLAY_WINDOW_SEC`（默认 300s），构造 verifier + dispatcher；wire 三 adapter + EchoAdapter fallback |
| Smoke | `cmd/a2a_smoke/main.go`：**5 旧 + 7 新 = 12 检查** |

### 错误码体系（Sprint 5.5 新增）

| 触发条件 | Error 字段 | gRPC code |
|---|---|---|
| `RegisterCard` `auth["ed25519"]` 非空但解析失败 | gRPC status msg `"F_006:invalid ed25519 public key"` | `codes.InvalidArgument` |
| `SendMessage` 发件方有 `auth["ed25519"]` 但 sig 空/坏/不匹配 | `MessageResponse.Error` `"F_007:..."` | — |
| `SendMessage` `\|now - ts_ms\| > ReplayWindow` | `MessageResponse.Error` `"F_008:..."` | — |
| `SendMessage` `recipient.Provider` 无对应 adapter 且 fallback 不接 | `MessageResponse.Error` `"F_009:..."` | — |
| `Stream` 任何验签 / 时间窗 / 路由失败 | gRPC status msg `"F_007/F_008/F_009:..."` | `codes.Unauthenticated` |
| Sprint 5 的 F_001–F_005 | 不变 | — |

### Canonical 签名形式

8 字段固定 JSON（`message_id`, `from_agent_id`, `to_agent_id`, `conversation_id`, `type`, `payload_b64`, `ts_ms`, `trace_id`），`payload_b64` 用 `base64.RawStdEncoding`，`signature` 字段在 canonical 里缺席。详见 [docs/06-A2A-canonical.md](./06-A2A-canonical.md)。

> 注：原 plan 设想 9 字段含 `provider`，但 proto `Message` 无该字段，
> 故 canonical 改为 8 字段；`provider` 路由由 `AgentCard.provider` 决定，与签名无关。

## 二、E2E 输出

```
$ go test ./apps/a2a-gateway/... -count=1
ok  	github.com/aicity/a2a-gateway/internal/a2asrv	0.155s
（47 PASS / 0 FAIL：13 verifier + 5 adapter + 2 registry + 9 service bufconn + 7 registry service 旧）

$ A2A_GRPC_ADDR=127.0.0.1:50061 ./bin/a2a-gateway.exe &
2026/09/05 13:13:42 a2a-gateway starting gRPC on 127.0.0.1:50061 (replay_window=300s)
2026/09/05 13:13:42 a2a-gateway ready (registry size=0)

$ ./bin/a2a_smoke.exe
[a2a_smoke] dial 127.0.0.1:50061 ...
[OK]   RegisterCard alice / bob
[OK]   Discover capability=chat → 2 cards
[OK]   SendMessage alice → bob delivered=true
[OK]   SendMessage alice → ghost delivered=false error=F_004
[OK]   Stream echo 3 messages
[OK]   generated alice2 ed25519 keypair
[OK]   RegisterCard alice2 (with ed25519 pubkey) accepted=true
[OK]   SendMessage alice2→bob2 (signed) delivered=true
[OK]   SendMessage alice2→bob2 (tampered) delivered=false error=F_007
[OK]   SendMessage alice2→bob2 (stale ts) delivered=false error=F_008
[OK]   RegisterCard invalid ed25519 pubkey → gRPC InvalidArgument F_006
[OK]   Stream alice2→bob2 (bad sig on 3rd) → Unauthenticated F_007

[OK] all 12 a2a_smoke checks passed against 127.0.0.1:50061
```

## 三、关键决策（与 plan 一致；新增 1 项）

| # | 决策 | 理由 |
|---|---|---|
| 1 | Canonical 用 8 字段而非 plan 设想的 9 字段 | proto `Message` 无 `provider` 字段；plan "不动 proto" 与 "canonical 含 provider" 互斥，故删 provider |
| 2 | `signature` 在 canonical 里**缺席**而非空串 | 防止自签递归；envelope 字段集合必须稳定 |
| 3 | opt-in 放行旧 agent（无 `auth["ed25519"]`） | 零摩擦迁移 Sprint 5 已注册 agent；Sprint 6+ 收紧为必签 |
| 4 | Stream 验签失败 → `codes.Unauthenticated` + 关流 | 流无法塞 MessageResponse.Error；Unauthenticated 语义明确 |
| 5 | EchoAdapter 作 fallback（兜底接 `provider==""` 与 `provider=="aicity"`） | Sprint 6 接 HTTP 真转发时只改 adapter_echo.go，service 零改动 |
| 6 | SDK 不 import a2a proto | 现有 sdk-go 只 stdlib + net/http；保持纯 stdlib 边界 |
| 7 | ReplayWindow 默认 300s + env `A2A_REPLAY_WINDOW_SEC` 可调 | 容忍 NTP 漂移 + 网络抖动 |

## 四、变更清单

### 新增
- `apps/a2a-gateway/internal/a2asrv/verifier.go`
- `apps/a2a-gateway/internal/a2asrv/verifier_test.go`（13 测试）
- `apps/a2a-gateway/internal/a2asrv/adapter.go`
- `apps/a2a-gateway/internal/a2asrv/adapter_echo.go`
- `apps/a2a-gateway/internal/a2asrv/adapter_test.go`（5 测试）
- `packages/sdk-go/signing.go`
- `docs/06-A2A-canonical.md`

### 修改
- `apps/a2a-gateway/internal/a2asrv/registry.go`：加 `validateEd25519` → F_006
- `apps/a2a-gateway/internal/a2asrv/service.go`：`NewService(reg, verifier, dispatcher)` 三参；SendMessage / Stream 走 verifier + dispatcher
- `apps/a2a-gateway/internal/a2asrv/registry_test.go`：加 2 测试（F_006 + 合法 pubkey）
- `apps/a2a-gateway/internal/a2asrv/service_test.go`：加 8 bufconn 集成测（签名/路由/Stream）
- `apps/a2a-gateway/cmd/main.go`：构造 verifier + dispatcher + 读 `A2A_REPLAY_WINDOW_SEC`
- `apps/a2a-gateway/cmd/a2a_smoke/main.go`：5 旧 + 7 新 = 12 检查
- `docs/SPRINT-5.md`：末尾追加 Sprint 5.5 链接 + canonical 规范链接

### 不动
- `packages/proto/a2a.proto`（`Message.signature` / `AgentCard.auth` 已就位）
- `go.work` / `apps/a2a-gateway/go.mod` / `packages/sdk-go/go.mod`（`crypto/ed25519` 是 stdlib，不入 go.mod）

## 五、后续 Sprint

按依赖顺序：
1. **Sprint 6**：a2a-restart HTTP gateway（gin adapter → gRPC client）；替换 adapter_echo.go 的 stub 为真 HTTP 转发
2. **Sprint 6+**：PG 持久化 agent_card + inbox；canonical 抽到 `packages/sdk-go/canonical.go`（Python/TS SDK 也对齐）
3. **Sprint 7+**：ACL + 跨城邦路由 + AgentCard 自签 / CA 模型
