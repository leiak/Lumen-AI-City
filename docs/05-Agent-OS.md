# Agent OS 内部架构

[← 返回目录](00-目录.md) | [← 04-API设计.md](04-API设计.md)

> 本文档对应原文档 §19：Agent OS 五模块循环（感知/记忆/情感/规划/行动/反思）+ Prompt 模板 + 性能与成本控制。

---

## 19. Agent OS 内部架构

### 19.1 五模块循环图

```
   ┌──────────────────────────────────────────────────────┐
   │                                                       │
   │   ┌──────────┐    ┌──────────┐    ┌──────────┐       │
   │   │Perception│ →  │ Memory   │ →  │ Emotion  │       │
   │   │(感知)    │    │ (记忆)   │    │ (情感)   │       │
   │   └──────────┘    └──────────┘    └──────────┘       │
   │        ▲                                │             │
   │        │                                ▼             │
   │   ┌──────────┐                    ┌──────────┐       │
   │   │ Actor  │    ←    ┌──────────┐ │ Planner  │       │
   │   │ (行动)  │          │Reflector│ │ (规划)   │       │
   │   └──────────┘    ←    └──────────┘ └──────────┘       │
   │                                                       │
   └──────────────────────────────────────────────────────┘
```

### 19.2 各模块详解

#### Perception（感知）
- **输入**：视野内事件、消息队列、当前 Tile 状态。
- **处理**：去噪、分类、优先级排序。
- **输出**：`Percept` 对象：`{ type, priority, payload, geo, ts }`。

#### Memory（记忆）
- **三层**：
  - **感官缓存**（Redis，TTL 5 分钟）：最近 10 个感知。
  - **情节记忆**（PostgreSQL + Milvus）：完整事件流，向量检索相关情节。
  - **语义记忆**（Milvus + Neo4j）：常识、原则、价值观。
- **重要性评分**：用 LLM 评估每条记忆的 importance（0-1），< 0.3 的会逐步被遗忘。

#### Emotion（情感）
- **六维向量**：Joy / Sadness / Anger / Fear / Disgust / Surprise（Plutchik 模型）。
- **触发**：感知 → 情感规则 + LLM 评估 → 更新情感向量。
- **影响**：情感影响 Planner 的目标权重（如"焦虑"使Agent 更倾向寻找"安全"位置）。

#### Planner（规划）
- **输入**：当前 Percept + Memory + Emotion + Goals。
- **机制**：
  1. 提取目标（短期 + 长期）。
  2. 生成候选动作（结合工具列表）。
  3. 用 LLM 评估每个候选动作的"效用"和"风险"。
  4. 选择最优动作。
- **批量优化**：每 5 秒一批处理，而非每次感知都决策。

#### Actor（行动）
- **调用 World Engine 的原子动作 API**。
- **失败回滚**：动作失败时通知 Reflector 反思。

#### Reflector（反思）
- **触发**：每 30 分钟、每完成一个事件、出现重大事件。
- **机制**：
  1. 提取最近情节。
  2. 让 LLM 生成"经验总结"。
  3. 写入长期记忆 + 调整 Planner 策略。

### 19.3 Prompt 模板示例

#### 决策 Prompt

```text
你是 {agent_name}，{persona_summary}。

【当前状态】
- 时间：{city_time}
- 位置：{tile_name}
- 健康：{health}
- 心情：{emotion_summary}
- 身上物品：{items}

【视野内事件】
{visible_events}

【最近记忆摘要】
{recent_memories}

【长期目标】
{long_term_goals}

【当前短期目标】
{short_term_goal}

请从以下候选动作中选择下一步：
{candidate_actions}

返回 JSON：
{
  "chosen_action": "action_id",
  "reason": "为什么选这个",
  "speech": "如果动作是 speak，这里填要说的话",
  "expected_outcome": "预期结果"
}
```

#### 反思 Prompt

```text
回顾 {agent_name} 过去 {time_window} 的经历：

{experiences}

请总结：
1. 做得好的 3 件事
2. 可以改进的 3 件事
3. 学到的 3 条经验
4. 对未来目标的影响

返回 JSON：
{
  "wins": [...],
  "improvements": [...],
  "lessons": [...],
  "goal_adjustments": [...]
}
```

### 19.4 性能与成本控制策略
- **批处理**：每 5 秒聚合感知，统一规划一次。
- **小模型优先**：日常决策用 `gpt-4o-mini` / `claude-haiku`，关键事件升级到 `gpt-4` / `claude-sonnet`。
- **决策缓存**：相同状态下的决策可缓存 1 分钟。
- **空载休眠**：无新感知时，Agent 进入低功耗状态（仅 30% 决策频率）。

### 19.5 Agent 决策状态机（State Machine）

> 每个 Agent 有一个明确的"决策状态"，防止无限循环 / 卡死。

```
       ┌──────────┐
       │  Idle    │ ← 初始 / 无新感知
       └────┬─────┘
            │ tick + new_percept
            ▼
       ┌──────────┐
   ┌──→│Planning  │ ← Planner 模块启动
   │   └────┬─────┘
   │        │ decision_made
   │        ▼
   │   ┌──────────┐
   │   │Executing │ ← Actor 调 World Engine
   │   └────┬─────┘
   │        │ result_received
   │        ▼
   │   ┌──────────┐
   │   │Reflecting│ ← Reflector 总结经验
   │   └────┬─────┘
   │        │
   │        ▼
   └────── Idle

   任意状态 → Idle：timeout / 强制中断 / Saga 失败
```

**状态转换触发条件**：
- `Idle → Planning`：Tick（5s）+ 有新感知（percept_queue 非空）。
- `Planning → Executing`：决策完成 + 通过校验（Schema、合规、置信度 ≥ 0.3）。
- `Executing → Reflecting`：动作完成（成功/失败均可，失败反思权重更高）。
- `Reflecting → Idle`：反思完成 / 反思超时（30s）。
- **任意状态 → Idle**：玩家手动退出、Saga 失败、Guardian 强切。

**防卡死的 3 道防线**：
1. 每个状态有 timeout（Planning 5s、Executing 10s、Reflecting 30s）。
2. 超时自动转 Idle + warn 日志 + 累计 3 次进入 §38.4 告警。
3. 同一状态连续重试 3 次仍卡 → 强制 Id le + record error + 进监控大盘。

### 19.6 五类决策日志事件

> §46 单行 JSON 日志在 Agent OS 中的具体落地：每个决策周期输出 5 类事件。

| 类型 | 触发时机 | 关键字段 | 采样率 |
|---|---|---|---|
| `perception` | Perception 模块输出时 | `agent_id, percept_type, priority, payload_hash` | 100%（低频时） |
| `planning` | Planner 决策完成时 | `agent_id, candidate_count, chosen, reason, llm_tokens` | 1%（写路径） |
| `execution` | Actor 调 World API 时 | `agent_id, action_id, action_type, result, latency_ms` | 100% |
| `reflection` | Reflector 总结时 | `agent_id, period, lessons_count, goal_changes` | 100% |
| `error` | 任意阶段异常 | `agent_id, stage, code, msg, stack, retry_count` | 100% |

**事件时序保证**：
- 每个事件带 `trace_id`（同一决策周期内一致，便于聚合 5 类事件）。
- ts 用毫秒精度，可按时间排序重建决策全貌。

**示例**（一次完整问候的 4 帧）：

```json
{"ts":1725270000000,"type":"perception","trace_id":"t_001","agent_id":"a_xxx","percept_type":"agent_entered","priority":7,"payload_hash":"sha256-xyz"}
{"ts":1725270003000,"type":"planning",  "trace_id":"t_001","agent_id":"a_xxx","candidate_count":3,"chosen":"speak","reason":"玩家主动打招呼，亲和力 9 倾向回应","llm_tokens":420,"model":"claude-haiku-4-5"}
{"ts":1725270004500,"type":"execution","trace_id":"t_001","agent_id":"a_xxx","action_id":"act_yyy","action_type":"speak","result":"success","latency_ms":120}
{"ts":1725270300000,"type":"reflection","trace_id":"t_002","agent_id":"a_xxx","period":"1h","lessons_count":3,"goal_changes":0}
```

**客户端拉决策链**（调试用）：`GET /agents/:id/decisions?trace_id=t_001` → 一次性返回 4 帧完整链路。

### 19.7 LLM 调用编排

> Agent OS 不是无脑调 LLM，而是有严格的"调用编排层"（组合→截断→缓存→升级/降级）。

#### 19.7.1 Prompt 动态组装

```python
def assemble_prompt(agent: Agent, percept: Percept) -> str:
    parts = [
        agent.persona.base_prompt,                    # 静态（人设 + 角色描述）
        f"当前时间：{city.clock()}",
        f"位置：{agent.current_tile.name}（{tile.weather}）",
        f"心情：{emotion_vector_str(agent.emotion)}",
        f"最近记忆：\n{format_memories(agent.recent_5_memories)}",
        f"候选动作：\n{format_actions(percept.candidates)}",
    ]
    return "\n\n".join(p for p in parts if p)
```

#### 19.7.2 Token 预算分配与截断

```python
BUDGET = {
    "system":   300,     # 人设 + 角色描述
    "memory":   800,     # 召回记忆
    "context":  500,     # 当前感知
    "action":   200,     # 候选动作
    "history":  400,     # 最近对话
    "response": 200,     # LLM 输出上限
}

def fit_context(parts: dict, budget: dict) -> dict:
    total = sum(token_count(parts[k]) for k in budget)
    if total <= sum(budget.values()):
        return parts
    # 超出预算：优先压缩 memory 与 history（保留 system 不动）
    parts["memory"]  = summarize(parts["memory"],  target=budget["memory"]  * 0.6)
    parts["history"] = summarize(parts["history"], target=budget["history"] * 0.5)
    return parts
```

#### 19.7.3 LLM 模型分级矩阵

| 决策类型 | 模型 | Avg Tokens (in/out) | 单价（USD/1M） | 占比 |
|---|---|---|---|---|
| 日常问候 / 行为 | `claude-haiku-4-5` | ~500 / 50 | $0.8 / $4 | 95% |
| 复杂规划 / 社交 | `claude-sonnet-4-6` | ~1500 / 300 | $3 / $15 | 4% |
| 关键时刻 / 危机 | `claude-opus-4-6` | ~2000 / 500 | $15 / $75 | <1% |
| 反思总结 | `claude-haiku-4-5` | ~3000 / 800 | $0.8 / $4 | 24次/天/Agent |

**模型升级触发**：
- Reflector 发现上次决策"严重失误" → 下一次同类决策升级到 sonnet。
- 玩家主动 `/think_deep` 命令 → 一次性 opus。
- 涉及交易/法务等高价值动作 → 默认 sonnet。

#### 19.7.4 Prompt 缓存（关键成本优化）

```
首次调用：组装 prompt → 算 SHA256 → 调 LLM → 答案 + cache_key（Redis 1min TTL）
1min 内相同 prompt 直接命中 cache，不调 LLM（0 token、0 钱）

命中率监控（每日报告）：
  命中率 > 40%  → 决策重复多，改为行为树
  命中率 < 5%   → prompt 设计冗余 / 状态变化太频繁
```

#### 19.7.5 并发与限流

- 同一 Agent 串行（同一时刻只有一个 LLM 调用，避免 context 错乱）。
- 不同 Agent 并发，受 L4 (per user) / L5 (全局) 限流（详见 §18.7.3）。
- LLM 调用必带 `trace_id`，配额扣减可按 trace_id 聚合。

### 19.8 决策可解释性（Explainability）

> 每个决策必须带"why + confidence + alternatives"，便于审计 / 调试 / 复盘。

#### 19.8.1 决策输出统一 Schema

```json
{
  "chosen_action": "speak",
  "why": "玩家刚进店主动打招呼，符合老李亲和力 0.9 的性格设定",
  "confidence": 0.85,
  "alternatives": [
    {"action":"ignore",  "score":0.10,"rejected_reason":"会显得冷漠"},
    {"action":"offer_coffee","score":0.05,"rejected_reason":"早上 6 点还没开门"}
  ],
  "factors_considered": [
    "personality.agreeableness=0.9",
    "emotion.joy=0.7",
    "memory.last_interaction=2d_ago"
  ],
  "trace_id":  "t_001",
  "model":     "claude-haiku-4-5",
  "tokens":    420,
  "latency_ms": 87
}
```

#### 19.8.2 四档信心度处理（防 NPC "乱说话"）

| Confidence | 处理 |
|---|---|
| ≥ 0.8 | 直接执行（不加修饰） |
| 0.5 - 0.8 | 执行 + 把 `why` 作为 NPC 对话前缀（"嗯... 应该这样吧..."） |
| 0.3 - 0.5 | 先暂停 0.5s，模拟"犹豫"动作，再执行 |
| < 0.3 | 改为"沉默 / 不回应"，避免 NPC 乱说 |

#### 19.8.3 解释的存储与回放

- 所有决策的 `why` + `confidence` 写入 `decision_log`（与 §46 的 JSON 日志合流）。
- 玩家通过 UI 点 NPC "你刚才为什么那样说？" → 调 `GET /agents/:id/recent_decisions?limit=20`。
- 内部调试：陪审团评审 AI 警察时（§36），可拉取任一决策的完整 reasoning 链。

### 19.9 心跳与时间槽（Tick 模型）

> Agent OS 不能"事件驱动"无脑循环，必须按固定 Tick 节拍批量处理感知。

#### 19.9.1 Tick 模型设计

```
Tick = 5s （可按 agent 配 1-30s）

每个 Agent 在 tick 上：
  1. 从 Redis 拉取 tick 期间累积的 Percepts
  2. 一次性调 Planner 决策（最多 1 次 LLM 调用/tick）
  3. Actor 执行 0-N 个动作
  4. 记录 5 类日志（§19.6）

Tick 期间内：
  - Perception 入 Redis 队列，不立即触发决策
  - 高优先级（priority ≥ 9）感知可打断当前规划
```

#### 19.9.2 时间槽分配

| 槽 | 间隔 | 用途 | 实现 |
|---|---|---|---|
| T1 Tick | 5s | 感知聚合 + 决策 | APScheduler |
| T2 短任务 | 1min | 移动完成检查、对话收尾 | APScheduler |
| T3 中任务 | 15min | 邻居关系更新、技能学习 | Celery beat |
| T4 长任务 | 1h | 反思总结、目标调整 | Celery beat |
| T5 日任务 | 24h | 记忆老化、关系衰减 | 凌晨 4 点 cron |

#### 19.9.3 Tick 动态调优

| 场景 | Tick 调整 |
|---|---|
| CBD 高密度区 | 缩短到 2s |
| 城市边缘 / 荒野 | 延长到 10s |
| NPC 离线（玩家离开视野） | 延长到 30s |
| NPC 死亡 / banned | 停掉，进"墓碑态" |
| 节日事件期间 | 区域内 NPC Tick -50% |
| 大停电等全局事件 | 全局 Tick +100% |

### 19.10 行为树（Behavior Tree）替代 LLM

> 不是每个决策都需要 LLM。背景板 NPC（§44 LOD=L0）用行为树即可，覆盖 80% 日常行为。

#### 19.10.1 行为树节点类型

```
Sequence  顺序节点：依次执行子节点，全部成功才算成功
Selector  选择节点：依次尝试，第一个成功即返回
Condition 条件节点：返回 true/false
Action    动作节点：执行具体操作（World API 调用）
Decorator装饰节点：包装其他节点（Loop / Inverter / UntilFail / UntilSuccess）
```

#### 19.10.2 示例：老李的"开店日常"行为树

```
Root (Selector)
├── Sequence                          ← 早上开店流程
│   ├── Condition: is_morning?        (6:00 - 10:00)
│   ├── Condition: shop_open?
│   ├── Action: greet_customer(visible_agent)
│   └── Action: serve_coffee(order)
├── Sequence                          ← 下午营业
│   ├── Condition: is_afternoon?
│   ├── Action: chat_with_neighbors()
│   └── Action: clean_tables()
├── Sequence                          ← 晚上打烊
│   ├── Condition: is_evening?
│   └── Action: close_shop()
└── Default                           ← 兜底
    └── Action: read_paper()
```

**何时升级到 LLM**（fallback 路径）：
- 玩家主动对话 → 行为树"对话"叶子节点触发 LLM（升级到 sonnet）。
- 玩家做出意外行为（行为树无匹配） → LLM 接管。
- Reflector 上周反思出新的"策略" → 行为树编译时插入新的 Condition。

#### 19.10.3 LLM vs 行为树成本对比

| 决策类型 | LLM 调用 | 行为树执行 | 节省 |
|---|---|---|---|
| 每 5s 一次 | $0.0005 | $0.00001 | 50× |
| 每日/Agent | $0.864（1728次） | $0.017 | 50× |
| 1 万 Agent/天 | $8,640 | $172 | **$8,468/天** |

**LOD 分级与 行为树/LLM 对应**：
- L0 背景板：纯行为树（覆盖 95% 日常动作）。
- L1 互动 NPC：行为树 + LLM 升级（仅对话/特殊动作）。
- L2 主角：全程 LLM（每 5s），叠加行为树作 safety net。

### 19.11 失败重试与降级

#### 19.11.1 五级失败分类

| 等级 | 触发 | 默认处理 | 升级路径 |
|---|---|---|---|
| L1 瞬时错误 | 网络抖、Redis 抖 | 重试 1 次，指数退避 | — |
| L2 LLM 失败 | 限流 / 超时 / Provider 5xx | 切到备用 Provider | 再失败 → 降到 haiku 模型 |
| L3 决策失败 | Schema 校验失败 / 置信度过低 | 改为"保持沉默"，记 warn | 不再升级 |
| L4 Agent 卡死 | 状态机 timeout / 同状态 3 次重试 | 强制 Idle，记 error | 进监控大盘 |
| L5 系统级 | 所有服务挂 / 整个集群 down | 进 §37 降级模式 | 全局不响应（503） |

#### 19.11.2 重试实现（tenacity 范例）

```python
from tenacity import (
    retry, stop_after_attempt, wait_exponential,
    retry_if_exception_type, before_sleep_log
)

@retry(
    stop=stop_after_attempt(3),
    wait=wait_exponential(multiplier=1, min=1, max=10),
    retry=retry_if_exception_type((TimeoutError, ConnectionError, RateLimitError)),
    before_sleep=lambda rs: log.warning(f"LLM retry {rs.attempt_number}"),
)
def call_llm(prompt: str, model: str = "claude-haiku-4-5") -> str:
    return anthropic.messages.create(
        model=model, max_tokens=200, messages=[{"role":"user","content":prompt}]
    )
```

#### 19.11.3 降级三步曲

```
LLM 调用失败 → 退到 行为树 决策
行为树无匹配 → 退到 历史决策 复用（最近 1h 内的相似决策 + confidence > 0.6）
都没有     → 退到 "安全默认"（"保持沉默" / "继续当前动作" / "原地不动"）
```

**禁止降级到"随机动作"**（会让 NPC 行为完全不可预测）。

### 19.12 自我反思（Reflection）机制详解

#### 19.12.1 反思触发条件

| 触发 | 频率 | 用途 |
|---|---|---|
| T4 定时（1h） | 全 Agent 强制 | 周期性总结 |
| 大事件完成 | 事件 end 后 5min | 锚定经验（节日、任务结束） |
| 严重错误 | LLM 反馈负面 / 玩家投诉 | 学习避免 |
| 玩家主动 | `/reflect <agent>` | 调试 / 调教（仅自己拥有的） |
| 关系剧变 | intimacy 变化 > 20 | 快速调整（告白、断交） |

#### 19.12.2 反思输入范围（**重要设计**）

- **不要把所有情节丢给 LLM**（cost 高 + noise 多）。
- 只取最近 N 个"重要"情节（importance ≥ 0.6）。
- 默认 N=20，超出截断到"过去 1h 摘要"。

```python
def select_reflection_episodes(agent, max_n=20):
    eps = agent.memories.filter(importance__gte=0.6).order_by(-created_at).limit(max_n*3)
    return cluster_and_dedup(eps)[:max_n]  # 去重相似情节
```

#### 19.12.3 反思输出 4 件套

```json
{
  "wins": [
    "今天和钟警官聊得开心",
    "给豆豆讲了独角兽故事，他笑了"
  ],
  "improvements": [
    "客人多时应该主动让位",
    "咖啡水温偶尔有点高"
  ],
  "lessons": [
    "亲和型 NPC 适合分享故事",
    "下午 3-4 点客人最多，需要提前备料"
  ],
  "goal_adjustments": [
    {"goal_id":"g_001", "old":"吸引 10 个新客人", "new":"提高回头客比例"}
  ]
}
```

**4 类输出的副作用**：
- `wins` / `improvements` → 写入玩家可查的"成长日志"（UI 展示）。
- `lessons` → 写入 Milvus 长期语义记忆（向量索引，影响未来决策）。
- `goal_adjustments` → 写入 Agent 的 Planner 策略权重（下次 planning 时生效）。

#### 19.12.4 反思成本控制

- 模型固定 `claude-haiku-4-5`（最便宜）。
- 默认每 1h 跑 1 次（1 天 24 次/Agent）。
- **每 Agent 每天反思成本 ≈ $0.04**，1 万 NPC ≈ **$400/天**（可控）。

#### 19.12.5 反思质量的"陪审团"评审

每月抽样 100 段反思，让 3 个独立 LLM 评分（一致性、洞察度、可行动性）。
- 平均分 < 3.5 → 反思 Prompt 升级（升级到 sonnet）。
- 平均分 > 4.5 → 可尝试拉长反思间隔到 2h。

### 19.13 多 Agent 协同决策

> 多人场景（§44 LOD=L2）下，Agent 之间需要"协商"而不是各自为政。

#### 19.13.1 协同模式矩阵

| 模式 | 触发场景 | 实现 | 成本 |
|---|---|---|---|
| 投票 | 多人提建议 | 简单多数票 | 0（无需 LLM） |
| 共识 | 高价值决策 | 多次 LLM 调用求"多数解" | 2-3× |
| 代表 | 选举"队长" | 临时 1 个 L1 角色决策 | 1× |
| 拍卖 | 资源竞争 | 第一价格密封拍卖 | 0（无需 LLM） |
| 议事 | 多人争议 | 主持人 + 发言列表 + 总结 | 3-5× |

#### 19.13.2 群决策 Prompt 示例

```
你是 {agent_names} 的临时代理，需要决定 {decision_topic}。

每人表达观点：
{agents_opinions}

综合判断，返回 JSON：
{
  "decision": "...",
  "reasoning": "...",
  "minority_opinions": [
    {"agent":"xxx", "view":"..."}
  ]
}
```

#### 19.13.3 协同决策的 4 道限流

| 限流 | 阈值 | 理由 |
|---|---|---|
| 单群组每小时 | ≤ 1 次群决策 | cost 控制 |
| 单群决策超时 | ≤ 30s | 防拖延 |
| 参与人数 | ≤ 6 人 | 超出易失控 |
| 群决策密度 | ≤ 5% / 全局决策 | 不允许协同刷屏 |

**30s 超时未达成共识的处理**：回退到"按 L1 角色的权重直接投票"。

### 19.14 决策回放（Replay）与调试

#### 19.14.1 回放的 3 大用途

1. **玩家投诉复盘**：投诉"NPC 突然发疯" → 拉决策链查因。
2. **单元测试**：录制决策 → 改代码 → 重放验证无回归。
3. **场景调试**：开发者插入"假设事件"测试 NPC 反应。

#### 19.14.2 录制格式

```
recording.bin = {
  "session_id":  "rec_xxx",
  "agent_id":    "a_xxx",
  "start_ts":    1725270000,
  "end_ts":      1725270900,
  "engine_ver":  "v2.1.3",
  "events": [
    {"ts":1725270001, "src":"redis","type":"perception","data":{...}},
    {"ts":1725270003, "src":"llm",  "type":"planning",  "data":{...}},
    {"ts":1725270005, "src":"world","type":"execution", "data":{...}},
    {"ts":1725270100, "src":"llm",  "type":"reflection","data":{...}}
  ]
}
```

#### 19.14.3 回放器实现（FastAPI + asyncio）

```python
class DecisionReplay:
    def __init__(self, recording):
        self.rec = recording
        self.index = 0

    async def replay(self, speed: float = 2.0):
        prev_ts = self.rec.start_ts
        while self.index < len(self.rec.events):
            evt = self.rec.events[self.index]
            await asyncio.sleep((evt.ts - prev_ts) / speed)
            prev_ts = evt.ts
            self.index += 1
            self._dispatch(evt)   # 支持断点 / 暂停 / 重置
```

#### 19.14.4 回放用 DevTools UI

- **时间轴**：可视化事件流，hover 查看事件详情。
- **断点**：可在任一事件停下，注入"假设事件"后继续。
- **比对**：同一 agent 在两个不同时间录制的决策可 diff（哪一次变了）。
- **导出**：支持导出 JSON / CSV 供事后分析。

---

[← 返回目录](00-目录.md) | [← 04-API设计.md](04-API设计.md) | [继续阅读：06-A2A协议.md →](06-A2A协议.md)