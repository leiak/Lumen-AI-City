# aicity-sdk-go

> **职责**：第三方 Agent 接入 AI City 联邦的 Go SDK
>
> **关键文档**：[docs/06-A2A协议.md §20.17](../../docs/06-A2A协议.md)

## 安装

```bash
go get github.com/aicity/sdk-go
```

## Canonical form（**Sprint 7+ 唯一规范**）

`packages/sdk-go/canonical.go::CanonicalBytes(s Signable)` 是整个 aicity 联邦中
**唯一**的 canonical 字节生成器。任何 server / SDK / smoke / 第三方 client 都
必须通过 `CanonicalBytes()` 生成签名原文，否则会与 server 端 verifier 失同步
→ 验签失败。

详细规范：[docs/06-A2A-canonical.md §五](../../docs/06-A2A-canonical.md#五变更协议)

不变量的 4 条约束：
1. **字段集合封闭**：canonical 仅含 8 个字段。新增字段必须同步修改 server
   (`a2asrv/verifier.go`) + SDK (`sdk-go/canonical.go`)，并在 commit 中标记
   `BREAKING:canonical:v2`。
2. **`payload_b64` 用 `base64.RawStdEncoding`**（无 padding）。
3. **`signature` 字段在 canonical 里恒为缺席**。
4. **`json.Marshal` 默认行为**：UTF-8，无 HTML escape，无 indent，字段顺序
   由 Go struct 字段声明顺序决定。

跨实现护栏：server 端 `verifier_test.go::TestVerifier_CanonicalBytes_MatchesSDK`
会 byte-equal 比对 SDK / server 输出，并跑真实验签回路。任何漂移立刻 fail。

## 用法

```go
package main

import (
    "log"
    "github.com/aicity/sdk-go/aicity"
)

func main() {
    client := aicity.NewClient("https://aicity.example.com/a2a", "sk-xxx")

    card := aicity.AgentCard{
        AgentID:      "my_agent_001",
        Name:         "My Agent",
        URL:          "https://my-agent.example.com",
        Provider:     "openclaw",
        Capabilities: []string{"dialogue"},
    }
    if err := client.RegisterCard(card); err != nil {
        log.Fatal(err)
    }

    peers, err := client.Discover("dialogue")
    if err != nil {
        log.Fatal(err)
    }
    log.Printf("found %d peers", len(peers))
}
```

签名 `Message`（与 server `canonicalBytes` 字节级一致）：

```go
import (
    "crypto/ed25519"
    "encoding/base64"
    aicity "github.com/aicity/sdk-go"
)

// 1) 生成密钥对
pub, priv, _ := aicity.GenerateKey()

// 2) 填 Signable 字段
s := aicity.Signable{
    MessageID:      "msg-001",
    FromAgentID:    "my_agent_001",
    ToAgentID:      "peer_agent",
    ConversationID: "conv-001",
    Type:           "request",
    PayloadB64:     base64.RawStdEncoding.EncodeToString([]byte("hello")),
    TsMs:           time.Now().UnixMilli(),
    TraceID:        "trace-001",
}

// 3) 拿 base64(stdEncoding) 签名串
sig, _ := aicity.SignMessage(priv, s)
msg.Signature = sig

// 4) 直接 SendMessage（server 端会验签）
```
