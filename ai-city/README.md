# AI城邦 (AI City)

> **基于真实或半虚构地图的 2.5D/3D AI 城邦平台**
> 地图 + 协议 + Agent 三位一体，让数万含人类数字分身与第三方联邦 Agent 在持续运行的世界中自治交互。

## 🚀 5 分钟上手

```bash
# 1. 克隆
git clone https://github.com/aicity/ai-city.git
cd ai-city

# 2. 安装工具链（Node 22 / pnpm / uv / Rust / Go / Docker）
./scripts/bootstrap.sh

# 3. 启动中间件（PG / Redis / Kafka / Neo4j / Milvus）
docker compose -f docker-compose.dev.yml up -d

# 4. 初始化种子数据
./scripts/seed-data.sh

# 5. 启动所有服务（按 Turbo 并行调度）
make dev

# 6. 访问
open http://localhost:3000        # Web 玩家端
open http://localhost:8080/health  # API Gateway
open http://localhost:8081         # Admin Portal
```

## 📁 仓库结构

```
ai-city/
├── apps/                # 15 个独立部署的微服务
├── packages/            # 13 个共享代码 / proto / schema / SDK
├── web/                 # 玩家端 Next.js 15
├── infra/               # Terraform + Helm + ArgoCD + Grafana
├── docs/                # 工程参考（14 篇）
└── scripts/             # 运维脚本
```

完整目录与说明见 [`docs/14-开发计划与工程骨架.md`](docs/14-开发计划与工程骨架.md)。

## 🛠️ 技术栈

| 层 | 选型 |
|---|---|
| 后端核心 | Python 3.12 + FastAPI |
| 性能层 | Rust 1.82+（World Engine / CDC） |
| API Gateway | Go 1.23 + go-kit |
| 前端 | Next.js 15 + React 19 + MapLibre GL JS |
| 数据层 | PostgreSQL 16 + Neo4j 5 + Milvus 2.4 |
| 消息总线 | Kafka 3.7 |
| 缓存 | Redis 7 |
| LLM | Claude Sonnet 4.6（主） + 多 Provider 兜底 |
| 部署 | K8s 1.30 + ArgoCD + KEDA |
| 观测 | OpenTelemetry → Grafana Tempo / Loki / Mimir |

## 📚 文档

| 角色 | 推荐阅读路径 |
|---|---|
| 产品 / 战略 | [01-愿景](docs/01-愿景.md) → [02-NPC](docs/02-NPC人设与剧本.md) → [07-MVP](docs/07-MVP与ADR.md) → [10-低成本规则](docs/10-低成本规则.md) |
| 后端架构师 | [01](docs/01-愿景.md) → [03-数据](docs/03-数据Schema.md) → [04-API](docs/04-API设计.md) → [05-Agent-OS](docs/05-Agent-OS.md) → [08](docs/08-架构优化v1.md) → [09](docs/09-架构优化v2.md) → [10](docs/10-低成本规则.md) |
| 前端工程师 | [01 §4 §5](docs/01-愿景.md) → [04 §18.3 §18.10](docs/04-API设计.md) → [08 §26 §35](docs/08-架构优化v1.md) → [12-BT编辑器PRD](docs/12-BT编辑器PRD.md) |
| AI 工程师 | [01 §6](docs/01-愿景.md) → [05 §19.5-14](docs/05-Agent-OS.md) → [08 §27 §32.5](docs/08-架构优化v1.md) → [09 §34 §36](docs/09-架构优化v2.md) → [10 §43-47](docs/10-低成本规则.md) → [13-Saga-DSL-RFC](docs/13-Saga-DSL-RFC.md) |
| DevOps / SRE | [03 §17.5 §17.7](docs/03-数据Schema.md) → [09 §37-41](docs/09-架构优化v2.md) → [14-开发计划](docs/14-开发计划与工程骨架.md) |
| 第三方 Agent 开发者 | [06-A2A协议](docs/06-A2A协议.md) → [04 §18.12](docs/04-API设计.md) → [11 §E.1 §E.2](docs/11-技术细节与玩法模式.md) |

## 🤝 贡献

参见 [`CONTRIBUTING.md`](CONTRIBUTING.md)。

- 所有重大决策必先开 ADR（[`docs/adr/`](docs/adr/)）
- 所有 PR 需通过 CI + 1 名代码 owner 审批
- 所有"应该没问题"的判断必须被测试**证明**没问题

## 📄 许可

Apache 2.0

---

> **哲学**：**先求"不崩"再求"好玩"**，工程化的最小规则从第一天就落地。
