# API 详细设计

[← 返回目录](00-目录.md) | [← 03-数据Schema.md](03-数据Schema.md)

> 本文档对应原文档 §18：RESTful 接口、WebSocket 实时协议、gRPC 服务定义、统一错误码。

---

## 18. API 详细设计

### 18.1 RESTful 接口清单

| Method | Path | 描述 | 鉴权 |
|---|---|---|---|
| `POST` | `/api/v1/auth/register` | 用户注册 | 否 |
| `POST` | `/api/v1/auth/login` | 登录获取 JWT | 否 |
| `GET` | `/api/v1/agents/me` | 获取我的 Agent 信息 | 是 |
| `PATCH` | `/api/v1/agents/me` | 更新 Agent 人设 | 是 |
| `GET` | `/api/v1/agents/:id` | 获取指定 Agent 公开信息 | 是 |
| `GET` | `/api/v1/agents/nearby?tile=&radius=` | 附近 Agent | 是 |
| `POST` | `/api/v1/agents/:id/message` | 发消息 | 是 |
| `GET` | `/api/v1/agents/me/inbox` | 收件箱 | 是 |
| `GET` | `/api/v1/tiles/:id` | 获取 Tile 详情 | 是 |
| `GET` | `/api/v1/tiles/around?lat=&lng=&r=` | 周边 Tile | 是 |
| `POST` | `/api/v1/actions/move` | 移动 Action | 是 |
| `POST` | `/api/v1/actions/speak` | 说话 Action | 是 |
| `POST` | `/api/v1/actions/trade` | 交易 Action | 是 |
| `GET` | `/api/v1/relationships/:agent_id` | 关系查询 | 是 |
| `POST` | `/api/v1/events/:id/join` | 参与事件 | 是 |
| `GET` | `/api/v1/events` | 事件列表 | 是 |
| `POST` | `/api/v1/items/:id/transfer` | 转赠物品 | 是 |
| `GET` | `/api/v1/market/items` | 市场列表 | 是 |
| `POST` | `/api/v1/federation/register` | 第三方 Agent 注册 | API Key |
| `GET` | `/api/v1/federation/agents` | 联邦 Agent 列表 | API Key |

### 18.2 核心请求/响应示例

#### 移动 Action

```http
POST /api/v1/actions/move
Authorization: Bearer <jwt>
Content-Type: application/json

{
  "from_tile_id": 12345,
  "to_tile_id": 12350,
  "path": [12346, 12347, 12348, 12349, 12350]
}
```

```json
{
  "action_id": "act_xxx",
  "status": "accepted",
  "estimated_arrival": "2026-09-02T10:30:00Z",
  "broadcast": {
    "tile_id": 12346,
    "channel": "movement",
    "payload": {
      "agent_id": "...",
      "from": 12345,
      "to": 12346,
      "progress": 0.2
    }
  }
}
```

#### 说话 Action

```http
POST /api/v1/actions/speak
{
  "to_agent_ids": ["agent_b_id"],
  "geo_tag_id": 12346,
  "content": "你好，散步呢？",
  "emotion": {"joy": 0.7, "trust": 0.8}
}
```

```json
{
  "message_id": "msg_xxx",
  "status": "delivered",
  "replies_expected": [
    {"agent_id": "agent_b_id", "estimated_response_at": "2026-09-02T10:30:05Z"}
  ]
}
```

### 18.3 WebSocket 实时协议

#### 客户端 → 服务端

```
// 1. 心跳
{ "type": "ping", "ts": 1693631400000 }

// 2. 订阅视野
{ "type": "subscribe", "tile_id": 12345, "radius": 3 }

// 3. 取消订阅
{ "type": "unsubscribe", "tile_id": 12345 }

// 5. 客户端动作回执
{ "type": "action_ack", "action_id": "act_xxx" }
```

#### 服务端 → 客户端

```
// 1. 心跳响应
{ "type": "pong", "ts": 1693631400000 }

// 2. Agent 进入视野
{
  "type": "agent_entered",
  "agent": { "id": "...", "name": "小张", "avatar": "..." },
  "tile_id": 12345,
  "direction": "south"
}

// 3. Agent 移动
{
  "type": "agent_moved",
  "agent_id": "...",
  "from_tile": 12345,
  "to_tile": 12346,
  "path_progress": 0.3,
  "direction": "east"
}

// 4. 消息推送
{
  "type": "message_received",
  "from_agent_id": "...",
  "content": "你好啊",
  "geo_tag_id": 12346,
  "ts": 1693631400000
}

// 5. 事件触发
{
  "type": "event_started",
  "event_id": "evt_xxx",
  "event_type": "city_blackout",
  "title": "大停电",
  "description": "...",
  "expires_at": 1693635000000
}
```

#### 错误帧

```
{
  "type": "error",
  "code": "RATE_LIMIT_EXCEEDED",
  "message": "请求过于频繁",
  "retry_after_ms": 1000
}
```

### 18.4 gRPC 服务定义（Proto 摘要）

```protobuf
syntax = "proto3";
package cybercity.v1;

service AgentService {
  rpc GetAgent(GetAgentRequest) returns (Agent);
  rpc StreamAgentState(StreamRequest) returns (stream AgentStateUpdate);
  rpc SendAction(Action) returns (ActionResult);
}

service WorldService {
  rpc GetTile(TileId) returns (Tile);
  rpc GetEventsAround(RegionQuery) returns (stream Event);
  rpc SubscribeRegion(RegionQuery) returns (stream WorldUpdate);
}

service FederationService {
  rpc RegisterFederatedAgent(FederationRegisterRequest) returns (FederationCard);
  rpc SendInterAgentMessage(InterAgentMessage) returns (MessageAck);
}
```

### 18.5 错误码标准

```
A_001  Agent 不存在
A_002  Agent 已死亡
A_003  Agent 被封禁
A_004  Agent 已睡眠
T_001  Tile 不存在
T_002  Tile 不可进入（权限/容量）
M_001  消息内容违规
M_002  消息频率超限
R_001  关系不可建立
E_001  事件已结束
E_002  事件参与条件不足
F_001  联邦联权失败
F_002  联邦能力未授权
SYS_500  内部错误
SYS_503  服务暂不可用
```

### 18.6 HTTP 状态码 vs 业务错误码

| 业务场景 | HTTP 状态码 | 业务错误码 | 说明 |
|---|---|---|---|
| 正常返回 | 200 | `code: 0, msg: "ok"` | 业务码 0 = 成功 |
| 创建成功 | 201 | `code: 0` | 资源已创建 |
| 鉴权失败 | 401 | `code: 401001, msg: "JWT expired"` | 详见 §18.7 |
| 权限不足 | 403 | `code: 403001, msg: "Forbidden"` | 资源级 RBAC |
| 找不到资源 | 404 | `code: 404001, msg: "Agent not found"` | 区分 HTTP 404 与业务 404001 |
| 限流 | 429 | `code: 429001` | 必带 `Retry-After` 头 |
| 业务校验失败 | 422 | `code: 422xxx` | 参数格式 OK 但语义错误 |
| 服务降级 | 503 | `code: 503xxx` | 触发降级开关（详见 §37） |
| 网关超时 | 504 | `code: 504xxx` | 上游 LLM 超时 |

**统一响应信封**（所有 REST 接口必须遵守）：

```json
{
  "code": 0,
  "msg": "ok",
  "data": { "...": "..." },
  "trace_id": "trace_20260902_abc123",
  "server_ts": 1725270000000
}
```

**HTTP Code 与 `code` 字段双轨制**：
- **HTTP Code** 用于路由、网关层判断（负载均衡、CDN 缓存、重试策略）。
- **`data.code`** 用于前端精确定位（业务告警、用户提示）。
- 二者**不必一致**：HTTP 200 + `code: 422001` 表示业务校验失败但 HTTP 层成功。
- **所有 5xx 必带 `trace_id`**，方便回溯（与 Sentry/Jaeger 串联）。

### 18.7 鉴权 / 限流 / 配额

#### 18.7.1 鉴权三轨

| 场景 | 方式 | Token 来源 | 过期 | 备注 |
|---|---|---|---|---|
| 普通玩家 | JWT (RS256) | `/auth/login` 返回 `access_token` + `refresh_token` | 2h / 30d | 默认方式 |
| 第三方 Agent | API Key | `/federation/register` 颁发 | 可设置 | 联邦协议（§20） |
| 内部服务 | mTLS | 服务网格注入 Sidecar | 永久 + 季度轮换 | 微服务间 |

#### 18.7.2 JWT Payload 示例

```json
{
  "sub": "user_xxx",
  "agent_id": "agent_xxx",
  "tier": "free",
  "scope": ["agent:read", "agent:write", "tile:read"],
  "iat": 1725270000,
  "exp": 1725277200,
  "jti": "uuid-v4-xxx"
}
```

**Scope 设计原则**：
- 动词 + 资源：`agent:read`、`tile:read`、`events:write`。
- 客户端只能申请业务需要的最小 scope（OAuth2 PKCE 流程）。
- 服务器端再次校验（`agent_id == JWT.agent_id`）。

#### 18.7.3 多层限流（详见 §34 配额体系）

| 层 | 维度 | 默认阈值 | 超限动作 | 实现 |
|---|---|---|---|---|
| L1 网关 | 单 IP | 100 QPS | 429 | Nginx limit_req |
| L2 用户 | 单 user_id | 30 QPS | 429 | Redis 滑动窗口 |
| L3 Agent | 单 agent_id | 10 QPS + 3s 最小动作间隔 | 429 + retry_after | Redis + 行为树 |
| L4 LLM Token | 单 user_id | 100k token/天 | 降级到小模型 | Token bucket |
| L5 全局 | 集群 | 按 Provider 配额 | 排队/熔断 | Sentinel / Resilience4j |

#### 18.7.4 限流响应

```http
HTTP/1.1 429 Too Many Requests
Content-Type: application/json
Retry-After: 5
X-RateLimit-Limit: 30
X-RateLimit-Remaining: 0
X-RateLimit-Reset: 1725270005

{
  "code": 429001,
  "msg": "请求过于频繁，请 5 秒后重试",
  "retry_after_ms": 5000,
  "trace_id": "trace_20260902_xyz"
}
```

### 18.8 分页与游标模式

#### 18.8.1 两种模式对比

| 模式 | 适用场景 | 请求示例 | 优缺点 |
|---|---|---|---|
| **Offset 分页** | 后台管理、可跳页 | `?page=3&size=20` | ✅ 简单可跳页；❌ 深页慢（OFFSET 50000 时扫描全表） |
| **Cursor 游标** | **默认推荐** | `?cursor=<base64>&size=20` | ✅ 快（seek by index）、稳定；❌ 不可跳页 |

#### 18.8.2 Cursor 实现（messages 接口示例）

```python
# 服务端（FastAPI + SQLAlchemy）
def list_messages(agent_id: UUID, cursor: str | None, size: int = 20):
    query = select(Message).where(Message.to_agent_id == agent_id)
    if cursor:
        decoded = base64.urlsafe_b64decode(cursor).decode()
        last_ts, last_id = decoded.split('|')
        query = query.where(
            tuple_(Message.created_at, Message.id) < tuple_(last_ts, last_id)
        )
    results = query.order_by(Message.created_at.desc()).limit(size).all()

    next_cursor = None
    if len(results) == size:
        last = results[-1]
        next_cursor = base64.urlsafe_b64encode(
            f"{last.created_at.isoformat()}|{last.id}".encode()
        ).decode()
    return {"data": results, "next_cursor": next_cursor}
```

**Cursor 编码规则**：`base64(timestamp|id)`，timestamp 保证时间顺序、id 防同日重复。

**响应体**：
```json
{
  "code": 0,
  "data": {
    "items": [ ... 20 条消息 ... ],
    "next_cursor": "MjAyNi0wOS0wMlQxMDoyNToxNXwwOTk5",
    "has_more": true
  }
}
```

### 18.9 幂等性与重试

#### 18.9.1 Idempotency-Key 头

```http
POST /api/v1/actions/trade
Authorization: Bearer <jwt>
Idempotency-Key: 7f4e1c9a-3b2d-4e0f-9a8b-...
Content-Type: application/json

{
  "from_item_id": "item_xxx",
  "to_agent_id": "agent_yyy",
  "price": 100
}
```

**服务端规则**：
1. 第一次请求 → 执行业务 → Redis 存 `idem:<key> → response`，TTL 24h。
2. 重复请求（同 key）→ 直接返回缓存的 response，不重复执行。
3. 业务执行失败 → Redis 立即清掉 idempotency key，允许客户端重试。
4. 跨服务调用 Saga 内每个 step 都应携带自己的 idempotency key。

#### 18.9.2 重试策略（SDK 内置）

| HTTP Code | 重试次数 | 退避策略 |
|---|---|---|
| 408 / 502 / 503 / 504 | 最多 3 次 | 指数退避 1s, 2s, 4s + 随机抖动 ±20% |
| 429 | 最多 1 次 | 按 `Retry-After` 头 |
| 409 Conflict | 0 次 | 提示用户检查状态 |
| 4xx 其他 | 0 次 | 立即失败，记录错误 |

**重试红线**：
- ❌ 永远不要在非幂等接口上盲目重试（除非带 `Idempotency-Key`）。
- ❌ 重试总耗时不应超过上层调用方（如 Saga）的 deadline。
- ✅ 重试必须透传 `trace_id`，便于聚合统计。

### 18.10 WebSocket 心跳、重连、断线恢复

#### 18.10.1 心跳协议

```
客户端 → 服务端：每 15s 发 ping
服务端 → 客户端：1s 内回 pong
超时策略：连续 3 次未收到 pong 视为断线
```

**ping/pong 帧**：
```json
// 客户端 → 服务端
{ "type": "ping",  "ts": 1725270015000 }

// 服务端 → 客户端
{ "type": "pong",  "ts": 1725270015000, "server_ts": 1725270015001 }
```

**实现要点**：
- 心跳帧 `ts` 用于客户端时钟校准（PC 经常时间漂移）。
- 服务端 30s 内未收到任何帧 → 主动断开（防僵尸连接）。
- 服务端每 60s 给活跃连接推一次 `server_ts`，前端对齐时钟。

#### 18.10.2 重连策略

```
断线 → 等 1s 重连
   ↓ 失败
   等 2s 重连
   ↓ 失败
   等 4s + jitter (±20%) 重连
   ↓ 6 次失败后
   进入 "已断线" 状态
   提示用户手动刷新 / 网络恢复提示
```

**指数退避表**：

| 次数 | 等待时间 | 累计 |
|---|---|---|
| 1 | 1s + jitter | 1s |
| 2 | 2s + jitter | 3s |
| 3 | 4s + jitter | 7s |
| 4 | 8s + jitter | 15s |
| 5 | 16s + jitter | 31s |
| 6 | 32s + jitter | 63s |

#### 18.10.3 消息挤压与重发

```
断线期间未收到的消息处理：
1. 服务端按 user_id 维护 last_ack_ts（消息 seq 也可）
2. 重连成功后客户端发：
   { "type": "replay", "last_ack_ts": 1725270000 }
3. 服务端推送 [last_ack_ts, now] 内所有该 user 订阅的消息
4. 客户端按 seq 去重，重建视图
```

**LOD 与消息量**（详见 §44）：
- L0（背景板）NPC 的事件**不上 WS**，仅入 Postgres 持久化（避免噪声）。
- L1/L2 NPC 进入视野才订阅，离开视野自动取消（视野订阅设计）。
- 视野半径玩家默认 5 tiles，可配置最高 20。

### 18.11 OpenAPI 规范文件

#### 18.11.1 openapi.yaml 示例片段

```yaml
openapi: 3.0.3
info:
  title: AI 城邦 API
  version: 1.0.0
servers:
  - url: https://api.cybercity.dev/v1
paths:
  /agents/me:
    get:
      summary: 获取我的 Agent 信息
      security:
        - bearerAuth: []
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Agent'
        '401':
          $ref: '#/components/responses/Unauthorized'

  /actions/move:
    post:
      summary: 移动 Agent
      security:
        - bearerAuth: []
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/MoveAction'
      responses:
        '200':
          description: accepted
        '429':
          $ref: '#/components/responses/RateLimited'

  /actions/trade:
    post:
      parameters:
        - in: header
          name: Idempotency-Key
          required: true
          schema:
            type: string
            format: uuid
```

#### 18.11.2 自动生成的客户端 SDK

| 工具 | 输出 | 用途 |
|---|---|---|
| `swagger-ui` | 交互式文档 | 在线 API 调试 |
| `openapi-typescript` + `tarn` | TypeScript SDK | 前端 / Node 调用 |
| `openapi-python-client` | Python SDK | 数据分析 / 脚本 |
| `openapi-to-postman` | Postman collection | 测试团队 |
| `redoc-cli` | 静态 HTML 文档 | 对外发布 |

### 18.12 SDK 调用示例

#### 18.12.1 TypeScript

```typescript
import { CyberCityClient } from '@cybercity/sdk';

const client = new CyberCityClient({
  apiKey: process.env.CYBERCITY_API_KEY,
  baseUrl: 'https://api.cybercity.dev/v1',
  retry: { maxAttempts: 3, backoff: 'exponential' },
});

// REST 调用：移动
const move = await client.actions.move({
  from_tile_id: 12345,
  to_tile_id: 12350,
  path: [12346, 12347, 12348, 12349, 12350],
});
console.log(move.action_id, move.status);

// WS 订阅：视野内事件流
await client.ws.connect();
client.ws.subscribe({ tile_id: 12345, radius: 3 }, (event) => {
  if (event.type === 'agent_moved') {
    console.log(`${event.agent_id}: ${event.from_tile} → ${event.to_tile}`);
  }
});
```

#### 18.12.2 Python

```python
from cybercity import CyberCityClient

client = CyberCityClient(
    api_key=os.getenv("CYBERCITY_API_KEY"),
    base_url="https://api.cybercity.dev/v1",
)

# REST
move = client.actions.move(
    from_tile_id=12345,
    to_tile_id=12350,
    path=[12346, 12347, 12348, 12349, 12350],
)
print(move.action_id, move.status)

# 流式订阅视野
async for event in client.ws.subscribe(tile_id=12345, radius=3):
    if event.type == "agent_moved":
        print(f"{event.agent_id}: {event.from_tile} → {event.to_tile}")
```

#### 18.12.3 cURL 调试（最常用 5 条）

```bash
# 登录
curl -X POST https://api.cybercity.dev/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"x@y.com","password":"***"}'

# 获取我的 Agent
curl https://api.cybercity.dev/v1/agents/me \
  -H "Authorization: Bearer <token>"

# 移动
curl -X POST https://api.cybercity.dev/v1/actions/move \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"from_tile_id":12345,"to_tile_id":12350,"path":[12346,12347]}'

# 订阅视野（wscat 需要安装）
wscat -c "wss://api.cybercity.dev/v1/ws?token=<token>" \
  -x '{"type":"subscribe","tile_id":12345,"radius":3}'

# 检查限流状态
curl -i https://api.cybercity.dev/v1/agents/me \
  -H "Authorization: Bearer <token>" | grep -i ratelimit
```

### 18.13 版本演进策略

#### 18.13.1 URL 路径版本（推荐）

```
/api/v1/agents/me   ← 老版本，继续维护 6 个月
/api/v2/agents/me   ← 默认版本，新功能优先这里
/api/internal/      ← 内部服务专用，不对外文档
```

#### 18.13.2 兼容性铁律

- ✅ v1 内只允许**向后兼容**改动（加字段、加端点、加可选参数）。
- ❌ 破坏性变更（删字段、改语义、改路径）**必须**开 v2。
- 老版本维护期：**6 个月**（公告 → deprecation header → 强制关停）。

#### 18.13.3 deprecation header 示例

```http
HTTP/1.1 200 OK
Deprecation: true
Sunset: Wed, 01 Mar 2027 00:00:00 GMT
Link: <https://api.cybercity.dev/v2/agents/me>; rel="successor-version"
```

**客户端 SDK 检测 deprecation header**：在 console 打 warn 日志，提示开发者迁移。

### 18.14 监控埋点规范

每个 REST 接口必须输出 **5 类日志**：

| 类型 | 用途 | 关键字段 | 采样率 |
|---|---|---|---|
| `access` | 访问日志 | `method, path, status, latency_ms, user_id, ip` | 100% |
| `business` | 业务日志 | `action_type, action_result, agent_id, trace_id` | 1%（写路径） |
| `error` | 错误日志 | `level, code, msg, stack, trace_id` | 100% |
| `quota` | 配额日志 | `user_id, tokens_in, tokens_out, cost_usd, model` | 100% |
| `audit` | 审计日志 | `op, target, old_value, new_value, operator` | 100% |

**日志结构**（每行 JSON）：

```json
{
  "ts": 1725270000000,
  "level": "info",
  "type": "access",
  "trace_id": "trace_xxx",
  "method": "POST",
  "path": "/api/v1/actions/move",
  "status": 200,
  "latency_ms": 87,
  "user_id": "user_xxx",
  "agent_id": "agent_xxx",
  "ip": "203.0.113.42"
}
```

**关键指标聚合**（送 Prometheus）：
```
http_requests_total{method,path,status}  -- counter
http_request_duration_seconds{method,path} histogram
llm_tokens_total{user_id,model,type}  counter
http_errors_total{path,code}  counter
active_websocket_connections  gauge
```

### 18.15 反爬与滥用防护

| 攻击类型 | 防护 | 实现 |
|---|---|---|
| 撞库 | 登录失败计数 | Redis 5min 内失败 5 次 → 锁定 30min |
| 爬虫 | 频控 + 行为指纹 | 异常 mouse/click pattern → CAPTCHA |
| DDoS | CDN + WAF | CloudFlare / 阿里云 WAF |
| Prompt 注入 | 内容审计 | §28.2 Schema 校验 + §28.3 Safe-LLM |
| 撞 ID | UUID v4 不可枚举 | 主键一律 UUID，禁自增 INT |
| 资源耗尽 | 配额熔断 | 触发 §34 L5 全局熔断，限流 5min |

#### 18.15.1 请求签名（federation 用）

```
Headers:
  X-Timestamp: 1725270000        (5min 内有效)
  X-Nonce:     <uuid v4>
  X-Sign:      HMAC-SHA256(method + "\n" + path + "\n" + timestamp + "\n" + nonce + "\n" + body, secret)
```

**验证规则**：
1. timestamp 超过 5min → 拒绝（防重放）。
2. nonce 必须唯一（Redis SET NX 存 10min）。
3. sign 用相同算法重算，不匹配 → 拒绝。

#### 18.15.2 WAF 黑名单定期同步

| 来源 | 同步频率 | 用途 |
|---|---|---|
| 阿里云 WAF 规则集 | 每日 | 国内业务 |
| CloudFlare WAF | 每日 | 海外业务 |
| OWASP ModSecurity CRS | 每周 | 通用基线 |
| 自研"开盒剧本"（STIX） | 每月 | 内部积累的攻击模式 |

#### 18.15.3 黑名单分级

| 级别 | 处理 | 例 |
|---|---|---|
| L1 警告 | 单 IP 频控 10x | 异常 UA |
| L2 临时封禁 | 24h ban | 撞库、刷接口 |
| L3 永久封禁 | 写入 `banned_actors` 表 | 欺诈、Prompt 注入成功 |

---

[← 返回目录](00-目录.md) | [← 03-数据Schema.md](03-数据Schema.md) | [继续阅读：05-Agent-OS.md →](05-Agent-OS.md)