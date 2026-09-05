# 06-A2A Canonical 签名形式（Sprint 5.5）

> 本文件钉死 `Message.signature` 字段的**生成**与**验证**两端必须共享的
> 字节序列。任何一端偏离 → 验签必失败；变更需双签 server + SDK 并升级 version。

## 一、不变量（4 条）

1. **字段集合封闭**：canonical 仅包含下表 8 个字段。新增字段必须
   同步修改 server (`a2asrv.verifier.go`) 与 SDK (`sdk-go/signing.go`)，
   并在 commit message 中标记 `BREAKING:canonical:v2`。
2. **`payload_b64` 用 `base64.RawStdEncoding`**（无 padding）。原因：跨语言 SDK
   互操作时 padding 最易漂；RawStdEncoding 是 RFC 4648 §3.2 的最短表达。
3. **`signature` 字段在 canonical 里恒为缺席**（不是空字符串，而是字段不存在）。
   验证端只对 envelope 做签名；envelope 不含 signature → 防自签递归。
4. **`json.Marshal` 默认行为**：UTF-8，无 HTML escape，无 indent。
   字段顺序由 Go struct 字段声明顺序决定（亦即 `tag` 顺序），
   两端必须保持完全一致的 struct 声明。

## 二、字段表（8 个，按 canonical 顺序）

| # | JSON key | 类型 | 来源 | 备注 |
|---|----------|-----|------|------|
| 1 | `message_id` | string | `Message.message_id` | UUIDv4 / ulid 自选 |
| 2 | `from_agent_id` | string | `Message.from_agent_id` | 必须已注册 |
| 3 | `to_agent_id` | string | `Message.to_agent_id` | 必须已注册 |
| 4 | `conversation_id` | string | `Message.conversation_id` | 空串亦合法 |
| 5 | `type` | string | `Message.type` | `"request" / "response" / "event"` |
| 6 | `payload_b64` | string | `base64.RawStdEncoding(Message.payload)` | **无 padding** |
| 7 | `ts_ms` | int64 | `Message.ts_ms` | 毫秒；用于重放窗口 |
| 8 | `trace_id` | string | `Message.trace_id` | 空串亦合法 |

**不在 canonical 中**（故意排除）：
- `signature`（防自签递归）
- `provider`（proto `Message` 无此字段；路由由 `AgentCard.provider` 决定）
- 任何后续新增字段（如 `ttl`、`priority`）—— 走 `version` 双签

## 三、完整示例

输入 `Message`：

```go
&a2av1.Message{
    MessageId:      "01JB8Z3R7N4F8K9P1X2Y3Z4W5V",
    FromAgentId:    "alice",
    ToAgentId:      "bob",
    ConversationId: "conv-001",
    Type:           "request",
    Payload:        []byte("hello"),          // 5 bytes
    TsMs:           1736112000000,
    TraceId:        "trace-abc",
    Signature:      "",                        // 签名前置空
}
```

生成 canonical bytes（服务端 `verifier.canonicalBytes`）：

```json
{"message_id":"01JB8Z3R7N4F8K9P1X2Y3Z4W5V","from_agent_id":"alice","to_agent_id":"bob","conversation_id":"conv-001","type":"request","payload_b64":"aGVsbG8","ts_ms":1736112000000,"trace_id":"trace-abc"}
```

说明：
- `payload_b64` = `base64.RawStdEncoding("hello")` = `"aGVsbG8"`（无 `=` 尾巴）
- `signature` 字段不出现
- 字段顺序固定；任何重排 → 验签失败

签名输出：

```go
sigBytes := ed25519.Sign(priv, canonicalBytes)
sigStr  := base64.StdEncoding.EncodeToString(sigBytes)  // 88 字符
// 写入 Message.signature 后即可 SendMessage
```

## 四、黄金向量（用于跨语言 SDK 对齐）

> 固定 priv/pub 对 + 固定 Message → 固定 sig。
> Sprint 6+ Python/TypeScript SDK 必须能复现以下 sig。
> 一旦漂移，先怀疑 SDK 的 `json.Marshal` 行为（HTML escape / 字段顺序）。

```text
priv hex  : 9d61b19deffd5a60ba844af492ec2cc44449c5697b326919703bac031cae7f60
            (示例值；实际测试用 ed25519.GenerateKey 现场生成)
pub hex   : d75a980182b10ab7d54bfed3c964073a0ee172f3daa62325af021a68f707511a

Message   : {message_id:"golden-1", from_agent_id:"alice", to_agent_id:"bob",
            conversation_id:"", type:"request", payload:[0x68 0x69],  // "hi"
            ts_ms:1700000000000, trace_id:""}

canonical : {"message_id":"golden-1","from_agent_id":"alice","to_agent_id":"bob","conversation_id":"","type":"request","payload_b64":"aGk","ts_ms":1700000000000,"trace_id":""}

sig base64: (每次运行不同 —— 因 priv/pub 现场生成)
```

跨语言 SDK 自检步骤：
1. 选定一组固定 priv/pub（hex 形式存进测试 fixture）
2. 构造上述 Message
3. 计算 `base64.RawStdEncoding(payload)` → 应得 `"aGk"`
4. `json.Marshal` envelope → 应得 canonical 字符串（**逐字节比对**）
5. `Sign(priv, canonical)` → 应得与 server 完全相同的 sig base64

## 五、变更协议

canonical 任何字段的增删改 = **破坏性变更**。流程：

1. PR title: `BREAKING:canonical:v2 <one-line summary>`
2. 双仓同时改：
   - `apps/a2a-gateway/internal/a2asrv/verifier.go` (`canonicalEnvelope`)
   - `packages/sdk-go/signing.go` (`Signable`)
   - `apps/a2a-gateway/cmd/a2a_smoke/main.go` (`signCanonical`)
3. 更新本文件 §四 的黄金向量（固定 priv/pub 让 sig 可复现）
4. `TestVerifier_CanonicalBytes_Deterministic` 必须更新断言

## 六、参考

- Sprint 5.5 设计：repo 当前 Sprint plan
- 实现位置：`apps/a2a-gateway/internal/a2asrv/verifier.go`
- SDK mirror：`packages/sdk-go/signing.go`
- 错误码：F_006 (RegisterCard pubkey 解析失败) / F_007 (signature) / F_008 (ts_ms)
