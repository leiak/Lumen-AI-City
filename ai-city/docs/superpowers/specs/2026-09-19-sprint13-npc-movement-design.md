# Sprint 13 — NPC 移动闭环设计

> 目标：让 1 个 NPC（王老板）在 3×3 地图上"真的会动"——agent-os 是行为引擎，
> 决定下一步去哪个 tile，发布移动事件，经 ws-gateway 广播，web WorldMap 实时挪动 NPC 圆点。
>
> 对齐 1.0 ROADMAP §一.B「NPC 行为引擎：每 30s 走 1 步」与 §二 1.0 边界（**不接 LLM**，纯脚本调度）。
> 单元测试为主（用户既定约束：暂不跑 acceptance / docker E2E）。

## 一、MVP 数据流（本 Sprint 落地）

```
agent-os MoveScheduler (每 30s/每 NPC)
   │  决定 target (tile_id, x, y)  ← 默认 chooser 在 home±1 tile 内随机走
   ▼
Redis aicity:npc_moved
   │  payload: {npc_id, tile_id, x, y, ts_ms}
   ▼
ws-gateway (多频道订阅 + Broadcast)  ───> web WorldMap
         │                                   │
         ▼                                   ▼
   NpcMoved 信封广播                 监听 aicity:npc_moved，按 npc_id 覆盖圆点坐标
```

MVP 里移动事件的**发布者是 agent-os**（与 npc_dialogue 相同的"行为引擎→Redis→网关→前端"架构）。
world-engine 不做改动、不承担权威坐标——见 §三 硬化项。

## 二、契约

### 2.1 `aicity:npc_moved` payload（agent-os 序列化）
```json
{ "npc_id": "npc_wang_boss_001", "tile_id": "tile_0_0", "x": 55.0, "y": 45.0, "ts_ms": 1700000000000 }
```
字段与 `aicity:player:moved` 对齐（tile_id/x/y/ts_ms），但主键是 `npc_id`。

### 2.2 信封（ws-gateway 下行）
`type = "npc_moved"`，payload 原样透传（与 player_moved/npc_dialogue 共用 `Envelope`）。
NPC 移动是全地图可见事件 → **Broadcast**（不是 SendToPlayer）。

### 2.3 文件改动面
| 层 | 文件 | 动作 |
|---|---|---|
| agent-os | `config.py` | +`redis_channel_npc_moved`、+`move_tick_seconds` |
| agent-os | `move_scheduler.py` | 新增：MoveScheduler + 默认 chooser |
| agent-os | `app.py` | lifespan 起 MoveScheduler background task |
| agent-os | `tests/test_move_scheduler.py` | 新增：publish 契约 / 过滤 disabled / 运动边界 / walk 驱动 |
| ws-gateway | `protocol/message.go` | +`TypeNpcMoved`、`NpcMoved` |
| ws-gateway | `config/config.go` | +`ChannelNpcMoved` |
| ws-gateway | `cmd/main.go` | 订阅 `aicity:npc_moved` 并广播 |
| ws-gateway | `cmd/main_test.go` | +npc_moved 契约测试 |
| web | `lib/ws-events.ts` | +`NpcMovedPayload`、事件常量、`isNpcMoved` 分派 |
| web | `components/Map/WorldMap.tsx` | 订阅事件，用 event 坐标覆盖 NPC 圆点 |

## 三、硬化项（不在本 Sprint，明确下一步）
- **world-engine 权威坐标**：agent-os 移动先调 world-engine 的 gRPC/REST Move，world-engine 落位置表 + 发 `npc_moved`，web 归属变化（tile→tile 的 npc_ids 迁移）走 `/v1/tiles` 拉取。
- **多 NPC/网格编排**：`walk` 步法字段已落地（王老板 `walk.tiles` 3 步走法，agent-os `default_chooser` 数据驱动）；下一步做多 NPC 各自 walk + 越界钳制。
- **碰撞/路径**：沿已有 `pathfinding.rs`（ComputePath）走，avoid building。
- **move 与 say 不重叠**：welcome/greeting 与移动节奏错峰（`move_tick_seconds` 与 `say_tick_seconds` 独立可配）。

> 已完成：walk 字段（npc-templates + npc_registry 解析 + move_scheduler 数据驱动 chooser）。

## 四、验收（单元级）
- agent-os：每 tick 每个 enabled NPC 恰好发一条 `npc_moved`（ts_ms 单调）；disabled 不发；payload 字段=2.1 五键。
- ws-gateway：`npc_moved` 信封到达所有连接（广播），不触发 SendToPlayer。
- web：WorldMap 收到事件后该 NPC 圆点坐标 = event.x/y（无事件时回退 tile 中心）。
- 默认 chooser：目标 tile 始终在 home±1 范围内，避免走出 3×3 网格。