# ADR-0001: Agent OS 使用 Python + FastAPI

> **状态**：Accepted
>
> **日期**：2026-09-03
>
> **决策人**：@tech-lead @ai-lead
>
> **影响范围**：`apps/agent-os/`, `apps/memory-service/`, `apps/saga-orchestrator/`

## 背景

Agent OS 是核心微服务，承担 NPC 五模块循环 + LLM 编排 + 决策日志。需要：
- 丰富的 AI / LLM 生态（Anthropic SDK、LiteLLM、Pydantic）
- 快速迭代（AI 实验密集）
- 类型安全（Pydantic 2）

## 决策

**采用 Python 3.12 + FastAPI + Pydantic 2 + uv（包管理）**。

## 备选方案

### 方案 A：Go + Gin
- ✅ 性能好，单实例支持 10K NPC
- ❌ AI 生态弱（无 LiteLLM 等价物），与 LLM Provider 集成需要自己写

### 方案 B：Rust + axum
- ✅ 性能最佳，硬实时
- ❌ AI 库几乎为零，迭代慢
- ❌ 团队学习成本高

### 方案 C：Node.js + Express
- ✅ TypeScript 类型
- ❌ LLM SDK 不成熟

## 影响

### 正面
- 直接用 Anthropic SDK、LiteLLM，集成零成本
- Pydantic 让数据结构变更安全
- uv 包管理极快

### 负面
- GIL 限制 → 用 multiprocessing / async 绕开
- 单进程 CPU 密集型任务不擅长 → 重计算下沉到 Rust（world-engine）

### 缓解
- Agent OS 异步 I/O 密集，CPU 不是瓶颈
- 关键路径（AABB 碰撞）下沉到 Rust
- 单实例 1000 NPC / 5min 内 benchmark 已通过

## 实施计划

- [x] pyproject.toml 创建
- [x] FastAPI 骨架
- [x] 五模块循环主类
- [x] LLM 调用包装
- [x] LOD 切换
- [x] 决策日志

## 验证

- [x] 单实例 100 NPC 跑通 1 小时
- [x] chat_turns 限制生效
- [x] Token 成本 < $200/day（MVP 阶段）

## 参考

- [docs/05-Agent-OS.md](../../docs/05-Agent-OS.md) §19
- [docs/11-技术细节与玩法模式.md §A.1](../../docs/11-技术细节与玩法模式.md)
