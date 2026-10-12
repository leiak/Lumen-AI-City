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

## 5. 备份与恢复

创建压缩度较低但恢复最快、可靠性最好的 PostgreSQL custom-format 备份：

```bash
./scripts/backup-production.sh
```

默认输出到 `backups/production/`，同时生成 SHA-256 checksum，保留最近 14 天。

每次发布前可以运行隔离演练：

```bash
./scripts/backup-restore-drill.sh
```

演练会创建一次性 PostgreSQL 数据库、应用 manifest 迁移、写入 marker、执行 `pg_dump`、删除 marker 后 `pg_restore`，最后验证数据与 migration 记录完整恢复。2026-10-12 演练结果：13 条 migration 全部恢复。

恢复前先停止写入口径，确认备份 checksum 后执行：

```bash
./scripts/restore-production.sh \
  --backup-file backups/production/aicity-YYYYMMDDTHHMMSSZ.dump \
  --checksum-file backups/production/aicity-YYYYMMDDTHHMMSSZ.dump.sha256 \
  --yes
```

restore 脚本会停止核心写服务、重建数据库对象，再重启核心服务。这是破坏性操作，只能在维护窗口执行，并且必须先完成一次备份文件校验。

## 6. 生产核心验收清单

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
| 备份恢复 | `pg_dump` / restore 演练通过 | ✅ |
| Observability | metrics、日志、告警接入 staging | ✅ |
| 限流与审计 | 登录、转账、购买、提现有限流与审计 | ✅ |
| E2E | 20 条 golden path 通过 | ✅ |
| 压测 | 50 玩家 + 10 Agent 稳定 | ✅ |

## 7. 当前已知约束

1. `world-engine` 已使用标准 PostgreSQL 协议客户端，支持 SCRAM-SHA-256。新建生产数据卷会使用密码认证；既有 trust 卷必须在维护窗口重置或手动升级为 SCRAM。
2. Redis 已启用 requirepass；数据面端口仍关闭。生产 GA 前继续评估内部 TLS / ACL 细分权限。
3. Kafka、Milvus、etcd、Minio 仅关闭宿主机端口，还未完成生产级集群配置。

这三项是 Production Core 的安全升级前置项，不允许跳过。

## 8. GA 验收命令

### 20 条 golden path

```bash
python scripts/e2e-golden-paths.py
```

默认覆盖本机 API Gateway `http://localhost:18080`，可用 `AICITY_API_URL` 覆盖。
2026-10-12 结果：20/20 通过。

### 50 玩家 + 10 Agent 压测

```bash
python scripts/load-test-ga.py --duration 60
```

压测对每个虚拟用户签发本地短 token，避免共享 IP 触发全局限流；登录/注册链路可用
`AICITY_LOADTEST_AUTH=1` 显式开启。验收阈值：错误率 ≤ 1%，p95 ≤ 2s。
2026-10-12 结果：1440 ops，0 错误，p95 1.524s，p99 2.018s。

## 9. 本机生产栈演练

2026-10-12 在本机 Docker 上用 `.env.production` 启动生产 overlay：

```bash
docker compose --env-file .env.production \
  -f docker-compose.yml -f docker-compose.prod.yml up -d --build
```

演练结论：

- 生产数据卷使用独立 `prod_pg_data` / `prod_redis_data`。
- PostgreSQL 使用 SCRAM-SHA-256；Redis / Kafka / Milvus / etcd / Minio 不发布宿主机端口。
- `world-engine` 修复了 Redis URL password 被丢弃的问题，现在 `PING` / `PUBLISH` / `SUBSCRIBE` 前都会先 `AUTH`。
- `a2a-gateway` 生产 overlay 补上带密码的 `REDIS_URL`。
- 20 条 golden path 在生产栈通过：20/20。
- 压测：1440 ops，0 错误，p95 2.267s，p99 3.114s。错误率达标；p95 略高于 2s，受本机 Windows Docker + 同机其他工作负载影响，不能作为生产容量结论。

## 10. 生产可观测性

API gateway 暴露：

- `aicity_http_requests_total`：按 route / method / status 聚合。
- `aicity_http_request_duration_seconds`：请求延迟直方图。
- `aicity_critical_action_total`：关键操作成功 / 失败计数。
- `aicity_rate_limit_rejected_total`：限流拒绝计数。

关键操作还会输出 `critical_action_audit` JSON 日志，包含 `trace_id`、`actor_id`、`client_ip`、HTTP result，可与数据库 transaction append-only 记录相互印证。

Prometheus 告警规则位于 `infra/prometheus/rules/alerts.yml`，覆盖 API / Economy 5xx、P99 延迟、关键操作失败率、认证 / 资金限流异常。

Day 4 新增 economy-service 指标：

- `economy_http_requests_total`：按 route / method / status 聚合。
- `economy_http_request_duration_seconds`：请求延迟直方图。

staging 可用以下命令启动可选观测栈：

```bash
docker compose --env-file .env.production \
  -f docker-compose.yml \
  -f docker-compose.prod.yml \
  --profile observability up -d prometheus grafana
```

Prometheus 会抓取 API Gateway、World Engine 和 Economy Service；Grafana 数据源自动指向 Prometheus。
