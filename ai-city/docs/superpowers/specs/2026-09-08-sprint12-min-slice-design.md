# Sprint 12 最小闭环设计 — NPC 主动 say + 玩家回复

> 日期：2026-09-08
> 状态：设计稿（待审）
> 范围：Sprint 12 T01+T02+T03+T04+T05+T06+T07
> 不在范围：T08/T09 welcome 剧本、T10 acceptance binary、T11 Playwright、T12 录屏/README/CHANGELOG、Sprint 11 整轮（proto EntityType / MoveEntity / BT）

## 〇、目标与判定

1.0 demo 在浏览器端可以演示的最小路径：

> 玩家登录 → 走到 `tile_0_0` → 王老板**主动 say** 一句（"来了您嘞！"）→ 浏览器右下角弹 NPCDialog 气泡（5 选项）→ 玩家点其中 1 项 → API-Gateway 查 `wang_boss.yaml::talk_tree` 拼出 NPC 的下一句 → 通过 `dispatcher.say` 走相同链路再次推给浏览器 → 看到 reply。

完成时验收：

- 手动 E2E：上述 5 步在浏览器跑通
- 单测：dispatcher.say、ws-gateway 多频道、talk_tree 解析、NPCDialog 组件 4 套测试全过
- 已 commit 的 demo 包 (`1.0-demo-package/quickstart.md`) 仍能跑（不动现有 demo 容器）

明确**不承诺**：

- 王老板**不会**主动迎客（无 welcome 剧本）
- 王老板**不会**移动（无 MoveEntity 实施）
- `acceptance_1_0` 二进制不能跑（T10 不在本范围）

## 一、架构

### 1.1 数据流

```
                 ┌────────────────┐
                 │ agent-os       │
                 │  dispatcher.   │
                 │   say()        │      (a) say 时钟
                 └───────┬────────┘
                         │
                         ▼ JSON envelope
                 ┌────────────────┐
                 │ Redis 频道     │  aicity:npc_dialogue
                 │  (pub/sub)     │
                 └───────┬────────┘
                         │
                         ▼
                 ┌────────────────┐         ┌────────────────┐
                 │ ws-gateway     │  WS     │ 浏览器         │
                 │ multi-channel  │ ──────► │  CustomEvent   │
                 │ Subscribe()    │         │  npc_dialogue  │
                 └────────────────┘         └───────┬────────┘
                                                   │
                                                   ▼
                                          ┌────────────────┐
                                          │ NPCDialog      │
                                          │ 气泡 + 5 选项  │
                                          └───────┬────────┘
                                                  │ 点选项
                                                  ▼
                                         POST /v1/npc/:id/talk
                                                  │
                                                  ▼
                                         ┌────────────────┐
                                         │ api-gateway    │
                                         │ talk_tree.go   │
                                         │ 读 wang_boss.  │
                                         │ yaml (mtime    │
                                         │ 缓存)          │
                                         └───────┬────────┘
                                                 │ reply
                                                 ▼
                                          再次走 dispatcher.say
                                          （同 a）形成回环
```

### 1.2 关键决策（已锁）

| 决策 | 取值 | 依据 |
|---|---|---|
| agent-os → ws 协议 | Redis pub `aicity:npc_dialogue` | 与 Sprint 9 `aicity:player_moved` 复用同总线 |
| envelope 字段 | `type=npc_dialogue`、`trace_id`、`ts_ms`、`payload.{npc_id, player_id, tile_id, say, options[]}` | 沿用 Sprint 12 runbook §一 字段表 |
| 玩家回复 API | `POST /v1/npc/:id/talk`，body `{player_id, choice_id}` | T05 runbook；不动 proto |
| `say` 触发源 | **agent-os 启动后周期性 fire**（不依赖 MoveEntity / 玩家 listener） | Sprint 11 整轮不实施，王老板必须主动说话；用 5s 一次的简单定时器，模板从 `wang_boss.yaml::say.greeting[]` 选 1 句 |
| 模板加载 | agent-os 进程启动时读一次 `packages/npc-templates/*.yaml` | 不引入 mtime watch，1.0 demo 不要求热重载 |
| 多频道订阅 | ws-gateway `Subscribe(ctx, rdb, channels, h, logger)` 重构当前 `PlayerMoved` | 1 个订阅者进程订阅 2 个频道；不再有 2 套独立函数 |
| WS 协议层 | `protocol.Envelope` 已支持任意 type（payload 是 `json.RawMessage`） | **无需改 ws-gateway protocol**，仅扩展 `TypeNpcDialogue` 常量 + `NpcDialoguePayload` 类型 |
| OCEAN schema 兼容 | 加 `say.welcome` + `talk_tree` 字段均为 optional | 1.0-sprint11-decisions.md 维持"向后兼容"原则；老模板继续 parse 通过 |
| Wang 老板 yaml 扩字段 | `enabled: true`、`npc_id: npc_wang_boss_001`、`say.greeting[3]`、`talk_tree{node_id, options[5], reply_map}` | 1.0-sprint11-tasks T09 + 1.0-sprint12-tasks T07 |
| `aicity_inbox` / `a2a` 通道 | 不动 | 与本范围无关 |

## 二、模块与文件改动

### 2.1 `apps/agent-os/`（Python 3.12）

```
apps/agent-os/
├── pyproject.toml                  # 改：fastapi 端口 8000→8084、删 LLM/kafka 依赖
├── Dockerfile                      # 改：EXPOSE 8084、CMD 用 main
├── README.md                       # 改：1.0 demo 段
├── src/agent_os/
│   ├── main.py                     # 新增（FastAPI app factory + uvicorn）
│   ├── app.py                      # 改：/healthz + /npc_loaded
│   ├── config.py                   # 改：HTTP_PORT=8084 + REDIS_URL + CHANNEL_NPC_DIALOGUE
│   ├── redis_pub.py                # 新增（手写 RESP；fire-and-forget + stats）
│   ├── npc_registry.py             # 新增（load yaml + list_enabled + get_template）
│   ├── action_dispatcher.py        # 新增（say() = JSON envelope + publish）
│   ├── say_scheduler.py            # 新增（5s tick → 选 greeting → say）
│   └── templates/                  # 软链 packages/npc-templates/（Dockerfile COPY 处理）
└── tests/
    ├── test_action_dispatcher.py   # 新增
    ├── test_redis_pub.py           # 新增
    ├── test_npc_registry.py        # 新增
    └── test_say_scheduler.py       # 新增
```

**`action_dispatcher.py::say` 签名**（与 runbook §一 一致）：

```python
async def say(
    self,
    npc_id: str,
    text: str,
    *,
    player_id: str | None = None,
    tile_id: str | None = None,
    options: list[dict] | None = None,
) -> None:
```

envelope 在 publish 前**必须**通过 `json.dumps(ensure_ascii=False)` 序列化。publish 失败仅 `logger.warning`，不抛、不重试（与 world-engine `RedisPub` 同款契约）。

**`redis_pub.py` 最小契约**：

- `RedisPub(url: str)` 解析 `redis://host:port[/db]`（从 world-engine `parse_redis_addr` 抄，不引 `redis-py`）
- `async publish(channel: str, payload: str) -> None`：fire-and-forget，1s connect timeout，写失败 → warn
- `stats() -> RedisStats`：6 原子计数器（与 world-engine 同名：`messages_published/connect_errors/write_errors/flush_errors/ping_success/ping_failure`）

**`say_scheduler.py`**：5s tick 调 `npc_registry.list_enabled()`，每个 enabled NPC 选 1 句 `say.greeting[]`（random.choice），调 `dispatcher.say(npc_id, text)`。**不**带 `options` 字段（welcome 剧本是后续 sprint 范畴）。

**`main.py`**：uvicorn 启动；`lifespan` 启动 scheduler（独立 task，参考 ws-gateway `appCtx` 模式），关闭时 `scheduler.stop()`。

### 2.2 `apps/ws-gateway/`（Go 1.23）

```
apps/ws-gateway/
├── internal/redis/
│   ├── subscriber.go              # 改：删除 PlayerMoved，引入 Subscribe(ctx, rdb, channels, h, logger)
│   └── subscriber_test.go         # 改：case 改测 multi-channel
├── internal/config/
│   └── config.go                  # 改：Channels []string（"aicity:player:moved,aicity:npc_dialogue"）
├── internal/protocol/
│   └── message.go                 # 改：增 TypeNpcDialogue 常量 + NpcDialoguePayload 类型
└── cmd/main.go                    # 改：wsredis.Subscribe(appCtx, rdb, cfg.Channels, h, logger)
```

**`Subscribe` 重构策略**（**关键陷阱**：避免破坏 7 个现有测试）：

- 新签名：`Subscribe(ctx, rdb, channels []string, b Broadcaster, logger)` — 用单一 redis pubsub 对象 `rdb.Subscribe(ctx, channels...)`（go-redis 9 原生支持多 channel 一次性 subscribe）
- 循环只保留 1 套：解析 → 包信封 → 广播
- 类型推断：`msg.Channel`（go-redis 提供）→ `protocol.TypePlayerMoved` / `TypeNpcDialogue`
- 删除老 `PlayerMoved` 函数（**只删这一个**，其它名字如 `runOnce`、`handle` 保留为内部 helper）
- `subscriber_test.go::TestPlayerMoved_*` 改名 `TestSubscribe_*`，case 仍 verify 2 个 channel 各自能收到

**`message.go::TypeNpcDialogue` 增量**：

```go
const TypeNpcDialogue = "npc_dialogue"

type NpcDialoguePayload struct {
    NPCID    string         `json:"npc_id"`
    PlayerID string         `json:"player_id"`
    TileID   string         `json:"tile_id"`
    Say      string         `json:"say"`
    Options  []DialogOption `json:"options"`
}
type DialogOption struct {
    ID   string `json:"id"`
    Text string `json:"text"`
}
```

**关键陷阱 — payload 字段是 `options: []DialogOption{}`（空数组）时 JSON marshal 后是 `[]`，不是 `null`**：agent-os 用 `options or []` 保证；ws-gateway 的 `json.RawMessage` 透传不再二次 marshal（避免精度漂移）。

### 2.3 `apps/api-gateway/`（Go 1.23）

```
apps/api-gateway/
├── internal/npc/
│   ├── talk_tree.go               # 新增：load yaml + mtime cache + 选择
│   └── talk_tree_test.go          # 新增：合法/非法 choice/NPC 不存在
├── internal/handlers/
│   └── npc.go                     # 新增：POST /v1/npc/:id/talk
├── internal/router/
│   └── router.go                  # 改：注册 npc handler
└── internal/npcstore/
    └── yaml_loader.go             # 新增：load NPC templates（与 agent-os 同款路径）
```

**`POST /v1/npc/:id/talk` 行为**：

1. 解析 body `{player_id, choice_id}`；缺失 → 400
2. `player_id` 在 PG `player` 表中查不到 → 401（与现有 `/v1/world/move` 一致鉴权）
3. `npc_id` 未在模板里 → 404
4. 模板无 `talk_tree` 或 choice_id 不在 options → 200 + `{npc_reply: "...", next_options: []}`，reply 兜底为 yaml 里 `default_reply`
5. 命中 `talk_tree[choice_id].reply` → 200 + reply + 该 reply 节点上的 `next_options`
6. reply 走 `dispatcher.say` 客户端调**不可行**（api-gateway 与 agent-os 不通），改为：
   - api-gateway 直接 `rdb.Publish(ctx, settings.RedisChannelNpcDialogue, envelopeJSON)` —— **需 api-gateway 也接 Redis**
   - 复用 `cmd/main.go` 已有的 `rdb` 实例（subscriber/player_moved.go 已用）
7. publish 失败 → 仍 200（fire-and-forget），但 `Response` 加 `dispatched: false` 字段给前端降级

**新增环境变量**：

```
NPC_TEMPLATES_DIR=/etc/aicity/npc-templates   # 默认 ./packages/npc-templates（dev）
REDIS_CHANNEL_NPC_DIALOGUE=aicity:npc_dialogue  # 与 agent-os 同
```

**`internal/npc/talk_tree.go` 数据结构**：

```go
type TalkTree struct {
    Initial   string                            `yaml:"initial"`
    Nodes     map[string]TalkNode               `yaml:"nodes"`
}
type TalkNode struct {
    Say         string         `yaml:"say"`
    Options     []DialogOption `yaml:"options"`
    ReplyMap    map[string]string `yaml:"reply_map"`     // choice_id -> node_id
    NextOptions []DialogOption `yaml:"next_options"`    // reply_node 的下一组选项
}
type DialogOption struct {
    ID   string `yaml:"id"    json:"id"`
    Text string `yaml:"text"  json:"text"`
}
```

yaml 加载：mtime 缓存（启动时 load；文件 mtime 变化则 reload；demo 不要求热重载，mtime 缓存用 sync.Map 即可）。

### 2.4 `web/`（Next.js 15）

```
web/src/
├── lib/
│   ├── ws-events.ts               # 改：增 NPC_DIALOGUE_EVENT + 分支
│   └── api.ts                     # 改：增 postNpcTalk(npcId, body)
├── components/
│   ├── NPCDialog.tsx              # 新增：状态机 + 5 选项 + 关闭
│   ├── NPCDialog.test.tsx         # 新增
│   └── Map/WorldMap.tsx           # 改：NPC 圆点 data-npc-id + onClick closest
└── app/city/
    └── page.tsx                   # 改：挂 <NPCDialog>
```

**`NPCDialog.tsx` 状态机**（最小）：

```
states:
  - hidden            初始；监听 aicity:npc_dialogue CustomEvent
  - showing-say       收到 envelope → 渲染 say + options
  - awaiting-choice   用户点选项 → postNpcTalk()
  - showing-reply     收到 reply envelope（type 仍 npc_dialogue, payload.npc_id == 选中的 + payload.say 是 reply）→ 渲染 reply + next_options
  - error             API 4xx/5xx → 显示 toast + 保留 say 视图
```

状态用 `useState`；transition 全部走事件。**关键陷阱**：reply envelope 与 say envelope 用同一个 `npc_dialogue` type 区分，仅靠 `payload.reply_to_choice_id` 字段（`null` 表示主动 say，非 null 表示 reply）。

**`ws-events.ts` 改动**：

```ts
export const NPC_DIALOGUE_EVENT = 'aicity:npc_dialogue';

function isNpcDialogue(msg: unknown): msg is WsEnvelope<NpcDialoguePayload> {
  if (typeof msg !== 'object' || msg === null) return false;
  const m = msg as Record<string, unknown>;
  if (m.type !== 'npc_dialogue') return false;
  const p = m.payload as Record<string, unknown> | undefined;
  return (
    typeof p === 'object' && p !== null &&
    typeof p.npc_id === 'string' &&
    typeof p.say === 'string'
  );
}

// 现有 onMessage 内：if isPlayerMoved(...) else if isNpcDialogue(...) else return;
```

**`WorldMap.tsx` 改动**：

- 现有 NPC 圆点 `<circle data-npc-id="...">`（runbook 标 T04a 要加的属性；查现状：当前 WorldMap 仅画 player dots + tile polygons，**NPC 圆点尚未渲染**——需在 tile 加载时按 `tile.npc_ids` 渲染 `<g data-npc-id="...">` 子组件）
- onClick 优先级：先 `e.target.closest('[data-npc-id]')`，命中 → `dispatchEvent(new CustomEvent('aicity:npc_dialogue', { detail: {payload: {npc_id, say: '...', options: [...]}, source: 'click' } }))`（**用同一 CustomEvent**让 NPCDialog 统一处理）
- "点空地移动"分支保留

**关键陷阱 — Click 拦截**：tile 中心常驻 NPC 圆点（r=3 viewBox 单位），直接 `page.locator('circle').click()` 仍会被 polygon 早返回分支挡掉。`data-npc-id` + `closest()` 已规避（Sprint 12 runbook §一 T04a 明确）。

### 2.5 `packages/npc-templates/wang_boss.yaml` 改动

增量字段（**全部 optional，向后兼容**）：

```yaml
npc_id: npc_wang_boss_001
enabled: true
home_tile_id: tile_0_0

say:
  greeting:
    - "来了您嘞！"
    - "今儿个想吃点啥？"
    - "喝杯茶？"
  welcome: []                # T09 才填，本范围留空
  default_reply: "嗯，您说的这事我得想想。"

talk_tree:
  initial: greet
  nodes:
    greet:
      say: "来了您嘞！"
      options:
        - {id: ask_business, text: "老板你这卖什么？"}
        - {id: ask_news,     text: "最近城里有什么新鲜事？"}
        - {id: ask_help,     text: "我想找个住处。"}
        - {id: just_chat,    text: "没什么，随便看看。"}
        - {id: leave,        text: "再见。"}
      reply_map:
        ask_business: business
        ask_news:     news
        ask_help:     help
        just_chat:    chat
        leave:        end
    business:
      say: "招牌红烧肉、酱肘子，外加二两老白干。要不要来一份？"
      options:
        - {id: order,     text: "来一份红烧肉。"}
        - {id: ask_price, text: "多少钱？"}
        - {id: back,      text: "我再看看。"}
      reply_map: {order: order_confirm, ask_price: price, back: greet}
    # ... 简版 4-5 个节点即可，1.0 demo 节奏快
```

**关键陷阱**：yaml `npc_id` 字段名**不**用 proto 的 `entity_id`，避免与 future proto 重命名冲突。

### 2.6 `packages/proto/OCEAN-schema.json` 改动

`properties` 顶层加：

- `npc_id` (string, optional)
- `enabled` (boolean, default false)
- `home_tile_id` (string, optional)
- `say` (object, optional, 含 `greeting/welcome/default_reply` 三个 array/string)
- `talk_tree` (object, optional, 含 `initial/nodes`)

`talk_tree.nodes.*.options[]` 子 schema 用 `oneOf [{id, text}]`。

## 三、错误处理与边界

| 场景 | 行为 |
|---|---|
| agent-os publish 失败 | warn，scheduler 继续 tick，丢这一条 |
| ws-gateway 收到坏 JSON envelope | warn + 丢，与 Sprint 9 同款 |
| api-gateway 收到未知 npc_id | 404 `{error: "NPC_001", detail: "npc not found"}`（独立错误码族，避免与 a2a F_xxx 混用） |
| api-gateway 收到未知 choice_id | 200 + `default_reply`（不报错，玩家体验好） |
| api-gateway PG player 不存在 | 401 |
| NPCDialog 收到 reply envelope 时当前已 hide | 忽略（用户主动关了） |
| NPCDialog POST 失败 | 错误 toast，**保留** say 视图，可重试 |
| `wang_boss.yaml` mtime 改 | api-gateway 30s 内 reload；agent-os 启动时读，不监听 |
| 多 player 同时触发同一个 NPC 的 reply | 各自独立 envelope，互不干扰 |
| 浏览器 close 浏览器 → agent-os 仍 say | envelope 进 ws-gateway，ws-gateway 无 client 订阅 → envelope 丢弃（fire-and-forget） |

## 四、测试

| 测试 | 范围 | 用例 |
|---|---|---|
| `tests/test_redis_pub.py` | 解析 + 序列化 + 失败重试 | 4 case：合法 URL 解析 / 非法 URL / publish 成功 / publish 失败仅 warn |
| `tests/test_action_dispatcher.py` | envelope 拼装 | 3 case：最小调用（npc_id+text）/ 全字段（带 options）/ publish 异常 → 不抛 |
| `tests/test_npc_registry.py` | yaml 加载 | 3 case：合法 yaml / 缺 npc_id / enabled=false 过滤 |
| `tests/test_say_scheduler.py` | tick 行为 | 2 case：5s tick 触发 say / list_enabled 空时不发 |
| `apps/ws-gateway/internal/redis/subscriber_test.go` | 多频道 | 4 case：2 频道都收 / 单 channel 关闭不影响另一 channel / 坏 JSON 丢 / 重连 backoff |
| `apps/api-gateway/internal/npc/talk_tree_test.go` | 解析 + 路由 | 5 case：合法 yaml / 非法 yaml 报错 / 未知 npc / 合法 choice / 非法 choice 走 default |
| `apps/api-gateway/internal/handlers/npc_test.go` | endpoint | 4 case：合法 POST → 200 / 缺 player_id → 400 / 缺 choice_id → 400 / PG player 缺失 → 401 |
| `web/src/components/NPCDialog.test.tsx` | 状态机 | 5 case：hidden / 收到 CustomEvent 进入 showing-say / 点选项 post / 收到 reply / close |
| `web/src/lib/ws-events.test.ts` | 派发 | 2 case：player_moved 仍触发 / npc_dialogue 触发新 event |

**总计**：32 个新测试。沿用各栈既有的 test 命令：`uv run pytest` / `go test ./...` / `pnpm vitest run`。

## 五、依赖与可执行性

### 5.1 新增 / 修改依赖

- `apps/agent-os/pyproject.toml`：删 `anthropic/litellm/sqlalchemy/aiokafka/opentelemetry/tenacity`；新增 `httpx>=0.27`（备用）；`pyyaml>=6.0`；`google/uuid` 已含在 stdlib
- `apps/ws-gateway`：无新增（go-redis 已支持多 channel）
- `apps/api-gateway`：新增 `gopkg.in/yaml.v3`（既有可能已在依赖里）
- `web/`：新增 `@aicity/ui-react` 引用（如已存在）或纯 React 写

### 5.2 容器 / 环境

- `docker-compose.yml`：暂不动（本范围不改容器编排；agent-os 现有 Dockerfile 改 EXPOSE 即可，dev 模式 `uvicorn` 直跑）
- 浏览器 → api-gateway CORS：现有 `CORS_ALLOWED_ORIGINS` 已含 `http://localhost:3000`
- 浏览器 → ws-gateway CORS：现有 `WS_VERIFY_ORIGIN=false` 维持 dev 默认

### 5.3 不动的现有契约

- `apps/world-engine` proto / gRPC：本范围不触发，**不增** `EntityType` / `MoveRequest.entity_type`
- `apps/a2a-gateway`：完全不动
- `packages/proto/world.proto`：不动
- `web/src/lib/ws.ts::onReconnect`：不动
- `web/src/lib/ws-events.ts::startWsBridge` 主流程：不动，仅加分支

## 六、未做但需要的后续 sprint 入口契约

| 接口 | 留给后续 sprint |
|---|---|
| 王老板 `MoveEntity(entity_type=NPC, npc_id, target_x, target_y)` | Sprint 13 实施 proto EntityType + world-engine server 分支 |
| Welcome 剧本（玩家首次进 tile_0_0 触发） | T08 + T09 |
| `acceptance_1_0` 5/5 binary | T10 + T11 + T12 |

## 七、回退 / 风险

| 风险 | 触发条件 | 缓解 |
|---|---|---|
| agent-os 旧五模块 loop 代码被新 dispatcher 覆盖 | 重构 loop.py | 保留 `AgentRuntime` 不动，新 `say_scheduler` 与 `ActionDispatcher` 是独立模块；如失败可注释 `app.lifespan` 中的 `scheduler.start()` 临时回退 |
| ws-gateway `Subscribe` 重构破坏 7 个现有 test | 改 `PlayerMoved` 签名 | 改名 `Subscribe` 后**批量更新** `subscriber_test.go` 调用点；如失败 git revert commit |
| api-gateway talk_tree yaml 解析与 agent-os 不一致 | 字段命名 / 嵌套层级 | api-gateway 端用独立 mirror struct（不 import agent-os 代码）；两套代码各自维护，yaml 是 single source of truth |
| `reply` envelope 误判为新主动 say | type 都是 `npc_dialogue` | 用 `payload.reply_to_choice_id` 区分（null/缺失 → 主动 say；非空 → reply） |
| `data-npc-id` 点击命中区域过小 | NPC 圆点 r=3 单位 | click handler 用 `closest('[data-npc-id]')` 命中 `<g>` 容器，命中范围 = 整个 g 的 bbox |
