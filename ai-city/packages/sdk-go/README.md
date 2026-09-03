# aicity-sdk-go

> **职责**：第三方 Agent 接入 AI City 联邦的 Go SDK
>
> **关键文档**：[docs/06-A2A协议.md §20.17](../../docs/06-A2A协议.md)

## 安装

```bash
go get github.com/aicity/sdk-go
```

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
