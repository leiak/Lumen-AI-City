# AI City Production Core GA

## 1. 目标

Production Core 是第一个可运营切片，而不是完整城市愿景。目标是把当前已验证的主链路变成可部署、可观测、可回滚、可验收的产品骨架。

## 2. GA 冻结范围

### 玩家链路

- 游客进入 `/city` 预览地图、建筑、玩家和 NPC。
- 登录、刷新恢复、退出。
- 钱包余额、Gold 转账、交易历史。
- NPC 对话、商品购买、库存。

### 创作者链路

- NPC / Saga 模板草稿。
- 草稿查询、发布、市场浏览、购买、下架。
- 创作者收益、提现、7 天趋势、作品归因。

### Agent 链路

- A2A 注册与发现。
- Agent 消息、NPC 行为树读取、脚本对话。
- Agent 移动。

### 生产链路

- 数据库版本化 migration。
- 生产环境变量收敛。
- 服务健康检查。
- 秘钥配置与默认开发凭据隔离。

## 3. 明确不做

- 细粒度经营报表和时间区间筛选。
- 高级行为树编辑器。
- 大规模 Agent 联邦。
- 完整 3D / LOD 城市。
- NPC 长期记忆、关系和声誉的完整玩法化。

## 4. 生产部署

复制模板并替换所有秘密值：

```bash
cp .env.production.example .env.production
```

部署：

```bash
./scripts/deploy-production.sh
```

部署流程会：

1. 校验 Compose 配置和必填环境变量。
2. 构建服务镜像。
3. 启动 PostgreSQL 和 Redis。
4. 执行 `scripts/migrate.sh`。
5. 启动生产服务。

Migration manifest 位于 `db/migrations/manifest.txt`。已应用的 migration checksum 改变会直接失败，避免对既有生产数据库做不可追溯的 schema 覆写。

## 5. 生产核心验收清单

| 项 | 验收标准 | 状态 |
|---|---|---|
| GA 范围 | 本文件功能冻结，不再加入新范围 | ✅ |
| 环境变量 | 生产 overlay 禁止开发默认秘密 | ✅ |
| Migration | 干净库和既有库均可跑通 | ✅ |
| 服务健康 | 所有 GA 服务有健康检查 | ✅ |
| 数据面安全 | PostgreSQL / Redis / Kafka / Milvus 不直接暴露宿主机端口 | ✅ |
| 前端 | API / WS 地址必须显式注入 | ✅ |
| CORS | API 与 WS 只允许显式配置的 Web origin | ✅ |
| CI | lint、后端测试、前端测试、构建通过 | ✅ |
| 备份恢复 | `pg_dump` / restore 演练通过 | ⚠️ Day 3 |
| Observability | metrics、日志、告警接入 staging | ⚠️ Day 3 |
| 限流与审计 | 登录、转账、购买、提现有限流与审计 | ⚠️ Day 3 |
| E2E | 20 条 golden path 通过 | ⚠️ Day 5 |
| 压测 | 50 玩家 + 10 Agent 稳定 | ⚠️ Day 5 |

## 6. 当前已知约束

1. `world-engine` 的手写 PostgreSQL 客户端目前只支持 trust 认证，因此生产 overlay 仍在容器网络内使用 trust。生产 GA 前必须替换为标准 PG 客户端或实现安全认证。
2. Redis 仍使用容器内无密码连接；数据面端口已关闭，但应在 Day 3 增加 ACL / TLS。
3. Kafka、Milvus、etcd、Minio 仅关闭宿主机端口，还未完成生产级集群配置。

这三项是 Production Core 的安全升级前置项，不允许跳过。
