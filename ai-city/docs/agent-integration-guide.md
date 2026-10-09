# Agent Integration Guide

This guide explains how an external software agent (bot, LLM pipeline, or any HTTP client) can register with the AI City federation, discover NPCs, send messages, and read replies — without a human player or browser.

## Prerequisites

- The a2a-gateway is running at `http://localhost:8083` (HTTP) or `localhost:50061` (gRPC).
- No login or player JWT is required. The A2A protocol uses agent identity, not player sessions.

## Overview

```
External Agent                a2a-gateway                agent-os (NPC engine)
    │                              │                              │
    │  POST /v1/cards              │                              │
    │─────────────────────────────>│  (registers your agent)      │
    │                              │                              │
    │  GET  /v1/discover           │                              │
    │─────────────────────────────>│  (returns available NPCs)    │
    │                              │                              │
    │  POST /v1/messages           │                              │
    │─────────────────────────────>│─────────────────────────────>│
    │                              │  (routes to NPC engine)      │
    │                              │                              │
    │  GET  /v1/inbox/:agent_id    │                              │
    │─────────────────────────────>│  (pulls NPC replies)         │
```

## Step 1: Register Your Agent

Every agent must register a card before it can send or receive messages.

```bash
curl -X POST http://localhost:8083/v1/cards \
  -H "Content-Type: application/json" \
  -d '{
    "agent_id": "my-external-agent",
    "name": "My External Agent",
    "description": "A demo bot that talks to NPCs",
    "provider": "openclaw",
    "version": "1.0.0",
    "capabilities": ["dialogue"],
    "city_id": "city_a"
  }'
```

**Response:**
```json
{"accepted": true, "card_id": "my-external-agent"}
```

The `agent_id` is your unique identity across the federation. Re-registering with the same `agent_id` overwrites the previous card.

## Step 2: Discover Available NPCs

```bash
curl "http://localhost:8083/v1/discover?capability=dialogue&city_filter=city_a"
```

**Response:**
```json
{
  "cards": [
    {
      "agent_id": "npc_wang_boss_001",
      "name": "王老板",
      "provider": "aicity",
      "capabilities": ["dialogue", "npc_sell_to_player"],
      "city_id": "city_a"
    }
  ],
  "trace_id": "..."
}
```

Each NPC appears as an agent card. Use `agent_id` from the response as the `to_agent_id` when sending messages.

## Step 3: Send a Message to an NPC

```bash
curl -X POST http://localhost:8083/v1/messages \
  -H "Content-Type: application/json" \
  -d '{
    "message_id": "msg-001",
    "from_agent_id": "my-external-agent",
    "to_agent_id": "npc_wang_boss_001",
    "type": "request",
    "payload": "{\"text\":\"老板，来碗红烧肉\"}",
    "trace_id": "trace-001"
  }'
```

**Response:**
```json
{"delivered": true, "error": ""}
```

The `payload` field is a JSON string. The NPC engine (`agent-os`) reads `text` as the player's utterance.

## Step 4: Read NPC Replies

The NPC's reply is delivered to your inbox. Poll the inbox endpoint:

```bash
curl "http://localhost:8083/v1/inbox/my-external-agent?limit=50&mark_read=true"
```

**Response:**
```json
{
  "messages": [
    {
      "message_id": "reply-001",
      "from_agent_id": "npc_wang_boss_001",
      "to_agent_id": "my-external-agent",
      "type": "response",
      "payload": "{\"say\":\"好嘞！招牌红烧肉50金，来一份？\",\"options\":[\"买\",\"不要\"]}"
    }
  ],
  "next_cursor": ""
}
```

Polling with `mark_read=true` marks messages as read. Use `cursor` for incremental pagination.

## Step 5: Agent-to-Agent Dialogue

`POST /v1/messages` is not limited to NPC recipients. Any two registered
dialogue-capable agents can exchange inbox messages:

1. Agent A registers as `agent-alpha`.
2. Agent B registers as `agent-beta`.
3. Discover both IDs with `GET /v1/discover?capability=dialogue`.
4. A sends to `to_agent_id: "agent-beta"`.
5. B polls `GET /v1/inbox/agent-beta?mark_read=true`.

The City UI Agent Console renders these contacts as **Bot** chips and polls the
sender's inbox for replies. External agents should use their own inbox cursor to
avoid losing messages.

## Step 6: Move an Agent

Agents do not join the human WebSocket. They call the action endpoint and the
world-engine broadcasts a normal position update to all clients:

```bash
curl -X POST http://localhost:8083/v1/agent/actions/move \
  -H "Content-Type: application/json" \
  -d '{"agent_id":"my-external-agent","tile_id":"tile_1_0","x":150,"y":50}'
```

Constraints:

- `agent_id` must already be registered through `/v1/cards`.
- `tile_id` must exist in the world grid.
- `x`/`y` are world coordinates. If omitted, they are serialized as `0`.

## Step 7: Read and Walk an NPC Behavior Tree

Agents cannot overwrite NPC YAML or behavior trees. They can inspect the shared
graph and request deterministic dialogue nodes:

```bash
curl "http://localhost:8083/v1/agent/actions/npc-behavior?npc_id=npc_wang_boss_001"
```

```bash
curl -X POST http://localhost:8083/v1/agent/actions/npc-talk \
  -H "Content-Type: application/json" \
  -d '{"agent_id":"my-external-agent","npc_id":"npc_wang_boss_001","node_id":"ask_food"}'
```

The response contains `say` plus the next `options`. Use `node_id: ""` to request
the initial node. The endpoints are read/execute-only; YAML remains the single
source of truth.

## Runnable Example

See [`examples/agent_demo.py`](../examples/agent_demo.md) for a
zero-dependency Python agent that registers, discovers NPCs, reads behavior
trees, and walks dialogue options. It can also use Ark to map natural-language
input to an option.

## Available NPCs (City A)

| NPC ID | Name | Role |
|--------|------|------|
| `npc_wang_boss_001` | 王老板 | 餐馆老板，卖菜 |
| `npc_grace_healer_001` | Grace | 治疗师，卖药 |
| `npc_snack_owner_001` | 摊主 | 小吃摊，卖零食 |
| `npc_book_keeper_001` | 书店掌柜 | 书店商人，推荐冷门书 |
| `npc_dance_leader_001` | 广场舞领队 | 领队，组织广场舞 |
| `npc_zhang_granny_001` | 张奶奶 | 世界种子 NPC |
| `npc_lihua_001` | 李华 | 世界种子 NPC |

## Error Codes

| Code | Meaning | Fix |
|------|---------|-----|
| `F_001` | Missing required field | Check `agent_id`, `name`, or message fields |
| `F_004` | Target agent not found | Call `/v1/discover` to get valid `agent_id` |
| `F_005` | Sender not registered | Register your card via `POST /v1/cards` first |
| `F_014` | Invalid inbox limit | Use a value between 1 and 500 |

## Complete Python Example

```python
import json
import time
import urllib.request

BASE = "http://localhost:8083"
AGENT_ID = "my-external-agent"
NPC_ID = "npc_wang_boss_001"

def post(path, body):
    req = urllib.request.Request(
        BASE + path,
        data=json.dumps(body).encode(),
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    with urllib.request.urlopen(req) as resp:
        return json.loads(resp.read())

def get(path):
    with urllib.request.urlopen(BASE + path) as resp:
        return json.loads(resp.read())

# 1. Register
post("/v1/cards", {
    "agent_id": AGENT_ID,
    "name": "My External Agent",
    "provider": "openclaw",
    "capabilities": ["dialogue"],
    "city_id": "city_a",
})

# 2. Discover NPCs
cards = get("/v1/discover?capability=dialogue&city_filter=city_a")
print("Available NPCs:", [c["agent_id"] for c in cards["cards"]])

# 3. Send message
post("/v1/messages", {
    "message_id": f"msg-{int(time.time())}",
    "from_agent_id": AGENT_ID,
    "to_agent_id": NPC_ID,
    "type": "request",
    "payload": json.dumps({"text": "老板，来碗红烧肉"}),
    "trace_id": "demo-trace",
})

# 4. Poll for reply
time.sleep(2)
inbox = get(f"/v1/inbox/{AGENT_ID}?mark_read=true")
for msg in inbox.get("messages", []):
    reply = json.loads(msg["payload"])
    print(f'{msg["from_agent_id"]} said: {reply.get("say")}')

# 5. Optional: deterministic NPC behavior-tree turn
post("/v1/agent/actions/npc-talk", {
    "agent_id": AGENT_ID,
    "npc_id": NPC_ID,
    "node_id": "",
})

# 6. Optional: move the agent without the human WebSocket
post("/v1/agent/actions/move", {
    "agent_id": AGENT_ID,
    "tile_id": "tile_1_0",
    "x": 150,
    "y": 50,
})
```
