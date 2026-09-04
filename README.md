# Lumen AI City — AI 城邦

> **基于真实或半虚构地图的 2.5D/3D AI 城邦平台**
> 玩家 + Agent 公民（含人类数字分身 + openClaw / workbuddy 等第三方联邦协议）
> 在持续运行的世界中自治交互。

## 📖 文档入口

| 文档 | 说明 |
|---|---|
| [`AI城邦-AI城市Agent平台-需求与架构设计.md`](AI城邦-AI城市Agent平台-需求与架构设计.md) | 顶层需求与架构设计稿（产品 + 战略） |
| [`docs/00-目录.md`](docs/00-目录.md) | 设计文档目录（01-14 共 14 篇，覆盖愿景 / 数据 / API / Agent OS / Saga 等） |
| [`ai-city/`](ai-city/) | **主实现 monorepo**：15 个微服务 + 13 个共享 package + Web 玩家端 |

## 🏗️ 实现入口

主项目在 [`ai-city/`](ai-city/) 下。核心信息：

- **技术栈**：Python 3.12 (FastAPI) + Rust 1.82 (World Engine / CDC) + Go 1.23 (API Gateway) + Next.js 15 (Web)
- **数据层**：PostgreSQL 16 + Neo4j 5 + Milvus 2.4 + Redis 7 + Kafka 3.7
- **15 分钟上手**：见 [`ai-city/README.md`](ai-city/README.md) `5 分钟上手` 段

## 📋 Sprint 进度

| Sprint | 状态 | 复盘 |
|---|---|---|
| 1 | ✅ done | — |
| 1.5 | ✅ done | [`ai-city/docs/SPRINT-1.5.md`](ai-city/docs/SPRINT-1.5.md)（world-engine REST + Redis Pub/Sub + api-gateway 订阅者链路） |
| 2 | ⏳ next | 见 SPRINT-1.5 §四 候选清单（`tile` PG 持久化、gRPC、Prometheus 接入） |

## 🤝 贡献

重大决策必先开 ADR（[`ai-city/docs/adr/`](ai-city/docs/adr/)）。
所有"应该没问题"的判断必须被测试**证明**没问题。

## 📄 许可

Apache 2.0

---

> **哲学**：**先求"不崩"再求"好玩"**，工程化的最小规则从第一天就落地。
