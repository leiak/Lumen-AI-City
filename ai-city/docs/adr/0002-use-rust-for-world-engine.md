# ADR-0002: World Engine 使用 Rust

> **状态**：Accepted
>
> **日期**：2026-09-03
>
> **决策人**：@tech-lead @rust-lead
>
> **影响范围**：`apps/world-engine/`

## 背景

World Engine 负责城市物理（Tile / 碰撞 / 移动 / 多玩家同步），要求：
- 单实例支持 10,000 NPC + 1,000 玩家
- TPS > 1000
- 硬实时（碰撞检测 < 0.1ms）

## 决策

**采用 Rust 1.82 + tonic (gRPC) + tokio**。

## 备选方案

### 方案 A：Python + asyncio
- ✅ 迭代快
- ❌ GIL + 性能瓶颈，单实例难破 100 NPC

### 方案 B：Go + gRPC
- ✅ 性能足够，生态熟
- ❌ 极端情况（AABB 碰撞 10K 次 / frame）仍比 Rust 慢 3-5 倍

### 方案 C：C++
- ✅ 极致性能
- ❌ 内存安全靠人工，团队规模小风险高

## 影响

### 正面
- 性能远超需求，预留 10x 余量
- 类型 + 内存安全，减少线上崩溃
- tonic gRPC 生态完整

### 负面
- Rust 学习曲线陡
- 招聘 2 名 Rust 工程师是关键路径风险

### 缓解
- Sprint 0 第 1 周内必须锁定 Rust 工程师
- 关键算法（AABB / 路径）抽到 `world_core` 库便于单测

## 实施计划

- [x] Cargo.toml 创建
- [x] tonic gRPC 服务骨架
- [x] Tile / AABB 数据结构
- [x] 单元测试覆盖核心算法

## 验证

- [ ] 单实例 benchmark：10K NPC + 1K 玩家 TPS > 1000（待 Sprint 4 压测）
- [ ] 单元测试覆盖率 ≥ 80%
- [ ] 24h 压测无内存泄漏

## 参考

- [docs/01-愿景.md §5](../../docs/01-愿景.md)
- [docs/11-技术细节与玩法模式.md §B.1](../../docs/11-技术细节与玩法模式.md)
