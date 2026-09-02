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

---

[← 返回目录](00-目录.md) | [← 04-API设计.md](04-API设计.md) | [继续阅读：06-A2A协议.md →](06-A2A协议.md)