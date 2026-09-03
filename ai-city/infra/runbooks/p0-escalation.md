# P0 告警值班手册（§41.9）

> **响应时间**：P0 告警触发后 5 分钟内必须有人响应。

## 告警清单与处置

### ALERT-001: LLM_Daily_Budget_Exceeded

**含义**：当日 Token 成本已超 $5,000 预算。

**立即处置**：
1. 触发自动降级：所有 L1 降回 L0（仅行为树）
2. 通知 LLM Provider 备份（OpenAI / 国产）
3. 在 Slack #incident 通告

**30min 内**：
- 调查异常原因（某 NPC 死循环？某剧本无 chat_turns 限制？）
- 暂时下线高成本 NPC / 剧本

### ALERT-002: SagaCompensationRate_High

**含义**：Saga 补偿率超过 10%。

**立即处置**：
1. 检查 saga-orchestrator / saga-worker 日志
2. 查看 Saga Dashboard 看哪个 step 失败最多
3. 检查下游服务（DB / Kafka / LLM）健康

**30min 内**：
- 若是 LLM 调用失败 → 切换 fallback model
- 若是 Kafka lag → 增加 consumer 副本
- 若是 DB 慢查询 → 排查 + 临时限流

### ALERT-003: NPC_DecisionLoop

**含义**：某 NPC 5min 内决策 > 5 次/s（死循环）。

**立即处置**：
1. 拉黑该 NPC（拒绝 LLM 调用，仅行为树）
2. 拉取 trace_id 看决策链
3. 检查 chat_turns 是否生效

**根本原因**：
- LLM 输出无法被解析 → 反复重试
- 行为树无限循环
- 外部刺激频率太高

## 升级流程

| 时间 | 角色 |
|---|---|
| 0-5min | 一线 on-call |
| 5-15min | Tech Lead |
| 15-30min | 工程 VP |
| 30min+ | CEO + 公关 |

## 联系

- 一线 on-call：PagerDuty 值班表
- Tech Lead：slack:tech-lead
- VP：slack:vp-eng
