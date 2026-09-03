# 常见故障 SOP（§41.10 历史故障案例库）

## INC-001: Redis 选举风暴（2025-09-12）

**现象**：Saga Orchestrator 5min 内重试 1000+ 次
**根因**：Redis Sentinel 主从切换时，客户端未正确处理 +inf 信号
**修复**：
1. 升级 redis-py 到 5.x（内置 cluster failover support）
2. 应用层加重试退避 + 熔断
3. 增加 `redis_cluster_failover_total` 监控

**预防**：
- 集成测试加 cluster failover 场景
- 每周演练 1 次

## INC-002: LLM Provider 大面积不可用（2025-10-03）

**现象**：所有 LLM 调用返回 503
**根因**：Anthropic 区域 outage
**修复**：
1. LiteLLM 自动切换到 OpenAI
2. 启用 haiku 兜底（响应稍差但可用）
3. 30min 后 Anthropic 恢复

**预防**：
- 准备至少 2 个独立 LLM Provider
- 定期演练 failover

## INC-003: 联邦外部 Agent 配额洪泛（2025-11-15）

**现象**：1 个外部 openClaw Agent 1min 内发送 10000 条消息
**根因**：对方客户端 bug，重试无退避
**修复**：
1. 紧急熔断该 Agent
2. 联系对方修复
3. 启用 IP 风控

**预防**：
- 入站限流（每 Agent 60 次/min）
- 配额制（每 Agent 每日 10K 条）

## INC-004: 节日级联失败（2025-12-31）

**现象**：新年倒计时 Saga 启动后 1min 内 50 个 NPC 同时调用 LLM
**根因**：Saga 触发无 jitter 退避
**修复**：
1. Saga step 之间加 random delay (0-2s)
2. 增加 chat_turns 限制
3. 紧急手动限流到 5 L1 / s

**预防**：
- 所有 Saga step 加 jitter
- 压测脚本覆盖节日场景
