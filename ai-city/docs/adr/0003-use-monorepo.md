# ADR-0003: 采用 Monorepo + Microdeploy 架构

> **状态**：Accepted
>
> **日期**：2026-09-03
>
> **决策人**：@tech-lead
>
> **影响范围**：仓库结构、CI/CD

## 背景

系统有 15+ 个微服务（不同语言：Python / Rust / Go / Next.js），需要：
- 共享代码（proto / schema / SDK）
- 独立部署（每服务独立伸缩）
- 统一 CI / 制品管理

## 决策

**采用 Monorepo（pnpm + turbo）+ Microdeploy（每服务独立 Helm chart + ArgoCD）**。

```
ai-city/
├── apps/       # 独立部署
├── packages/   # 共享代码
├── web/        # 玩家端
├── infra/      # IaC
└── docs/       # 工程参考
```

## 备选方案

### 方案 A：Multi-repo
- ✅ 服务自治
- ❌ 跨服务变更要开多个 PR，proto 同步困难
- ❌ 共享代码版本管理复杂

### 方案 B：Monolith + 模块
- ✅ 简单
- ❌ 单体无法满足 10K NPC + 1K 玩家
- ❌ LLM / 物理不能用同一语言

## 影响

### 正面
- 一处 proto 变更，所有服务立即可见
- 共享 SDK 同步发布
- CI 一次构建整库测试

### 负面
- 仓库体积膨胀（需要 sparse checkout）
- Turbo cache 配置有学习成本

### 缓解
- 启用 Git LFS 处理大文件
- 关键路径服务（Agent OS）单独 pipeline
- CODEOWNERS 精确到目录

## 实施计划

- [x] 仓库结构创建
- [x] Turbo 配置
- [x] CODEOWNERS 配置
- [x] 跨服务 CI

## 验证

- [ ] 仓库克隆 < 2min（sparse checkout）
- [ ] CI 单服务 PR < 5min
- [ ] 共享 SDK 变更 24h 内所有下游同步

## 参考

- [docs/11-技术细节与玩法模式.md §A.2](../../docs/11-技术细节与玩法模式.md)
