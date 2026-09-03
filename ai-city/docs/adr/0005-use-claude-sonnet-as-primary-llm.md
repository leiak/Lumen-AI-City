# ADR-0005: 主 LLM 选择 Claude Sonnet 4.6 + 多 Provider 兜底

> **状态**：Accepted
>
> **日期**：2026-09-03
>
> **决策人**：@ai-lead @tech-lead
>
> **影响范围**：`apps/agent-os/`, 整个 LLM 调用链

## 背景

NPC 对话 / 决策 / 反思都需要 LLM。要在成本 / 质量 / 可用性之间平衡。

## 决策

**主模型：Claude Sonnet 4.6**
**降级模型：Claude Haiku 4.5**
**灾备：OpenAI GPT-4o / 国内 Qwen / DeepSeek**

通过 LiteLLM 统一接口，按需切换。

## 备选方案

### 方案 A：只用 Claude Sonnet
- ✅ 质量最好
- ❌ 单 Provider 故障 → 全瘫（参见 INC-002 历史案例）

### 方案 B：自托管开源模型
- ✅ 数据可控
- ❌ NPC 对话质量差太多
- ❌ GPU 成本不可控

### 方案 C：多家混用 + 投票
- ✅ 最鲁棒
- ❌ 成本 3x，质量上限被拉低

## 影响

### 正面
- Sonnet 4.6 在 NPC 长对话 / 多步推理上质量明显领先
- Haiku 降级到成本 < 1/10
- OpenAI / 国产模型可在区域 outage 时启用

### 负面
- 多 Provider 增加 SDK 复杂度
- LiteLLM 增加一层网络（< 5ms）

### 缓解
- LiteLLM 统一抽象，调用方不感知
- 主备切换通过 chat_turns + importance 触发，无需重启

## 实施计划

- [x] LiteLLM 部署
- [x] Primary / Fallback 配置
- [x] 按 importance / chat_turns 动态路由
- [ ] 灾备演练（每季度）

## 验证

- [ ] 单 Provider outage 30min 内自动切换（参见 INC-002）
- [ ] NPC 对话质量盲评 ≥ 4.0/5
- [ ] 日成本 ≤ $5,000（§10 红线）

## 参考

- [docs/05-Agent-OS.md §19.7](../../docs/05-Agent-OS.md)
- [docs/10-低成本规则.md §42](../../docs/10-低成本规则.md)
- [docs/09-架构优化v2.md §34.4](../../docs/09-架构优化v2.md)（多 Provider 兜底）
