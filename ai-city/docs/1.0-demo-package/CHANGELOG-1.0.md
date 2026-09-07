# CHANGELOG-1.0

> AI 城邦 1.0 release notes
> 范围：M3 MVP demo 闭环
> 工时：Sprint 11 (14d) + Sprint 12 (20d) = **34d ≈ 6.8 周**

---

## 1.0 是什么

**1.0 = M3 MVP 最小切片**：stakeholder 5 分钟看完能说"我懂"的可演示闭环。

### 核心场景
玩家进入 3×3 城邦 → 看到 1 个 NPC（王老板）走动 → 玩家走近 → **NPC 主动打招呼** → 玩家点选项回复 → **NPC 接着聊** → 多 tab 实时同步 → 5 步自动化验收。

### 关键差异化
**NPC 主动 say** —— 玩家走近时 NPC 主动打招呼，不是玩家问 NPC 才回答。这是 1.0 最核心差异化（vs 传统 RPG NPC 站桩不动）。

---

## 新增能力

### agent-os 行为引擎（Python BT runtime）
- 位置：`apps/agent-os/`
- 行为树节点：Sequence / Selector / Action（3 节点）
- NPC 心跳 tick（默认 1s）+ 玩家 listener
- dispatcher 模式：`say / move / wait` 3 种 action
- **不接 LLM**（1.0 永远不做）

### 1 NPC 模板（王老板 wang_boss.yaml）
- OCEAN 五维人格（Openness / Conscientiousness / Extraversion / Agreeableness / Neuroticism）
- Speech style（tone / dialect / catchphrase）
- Backstory（500-2000 字）
- Schedule（按时间表活动：6 点起床 / 10 点开门 / 22 点打烊 / 23 点听京剧）
- **talk_tree**（2 层对话分支，5 root 选项 + 多 leaf 选项）
- **say.welcome[]**（新玩家入场 3 句）

### 点 NPC 弹气泡（NPCDialog.tsx）
- 位置：`web/src/components/NPCDialog.tsx`
- 状态机：`empty / showing / waiting-reply / showing-reply`
- 5 选项渲染 + 点击选项调 API + reply 显示
- 占位符 UX（点 NPC 但 NPC 没说话时）

### 1 条剧本（welcome storyline）
- 位置：`packages/npc-templates/wang_boss_storylines/welcome.yaml`
- 触发：玩家首次进入 tile_0_0 且 in_range
- 步骤：say welcome 句（3 句随机）→ 显示 talk_tree.root 选项
- **First-login 进程内 set**（重启丢，1.0 demo 接受）

### 2 个 1.0 必新 endpoint

#### `POST /v1/npc/:id/position`
- 玩家查询 NPC 当前位置（用于 hover 显示名字）
- Sprint 11 加 world-engine `MoveEntity` RPC（EntityType enum: PLAYER / NPC）

#### `POST /v1/npc/:id/talk`
- 玩家回复 NPC：body `{"choice": "ask_business"}`
- 查 `wang_boss.yaml::talk_tree[choice]` → 返 reply + next_options
- **不持久化对话历史**（1.0 简化）

### acceptance_1_0 自动化验收 binary
- 位置：`apps/a2a-gateway/cmd/acceptance_1_0/main.go`
- 5 步骤：login / walk / approach NPC / NPC主动say / 玩家回复
- 每步有 hint 表（hardcode）：失败时自动选 hint 输出
- 退出码：0=PASS / 1=步骤失败 / 2=setup失败 / 3=bad args / 4=中断
- `-verbose` / `-keep-alive` / `-ci-mode` 模式

### WS 多频道（dispatcher.say 走 Redis publish）
- dispatcher.say 改实现：log → `redis_client.publish("aicity:npc_dialogue", json.dumps(envelope))`
- ws-gateway 接 `aicity:player:moved` + `aicity:npc_dialogue` 两频道
- web 监听 `aicity:npc_dialogue` CustomEvent → 自动开 NPCDialog

### 8 容器 compose
- 已有 7 容器保留：postgres / redis / world-engine / api-gateway / ws-gateway / web / a2a-gateway
- 新增：agent-os（Python 3.12 + FastAPI + grpcio + redis）

---

## 显式 out-of-scope（1.0 不做，2.0+ 才做）

> 这些是 stakeholder 可能问"为什么不做"的清单。每次都用这一节回。

| 类别 | 项 | 为什么不做 | 2.0 何时做 |
|---|---|---|---|
| AI | LLM 接入 | 成本高 + 不可控 + 合规 | 2.0 启动条件 |
| AI | NPC 智能生成对话 | 同上 | 2.0 |
| 数据 | Milvus 记忆 | 1.0 不需要记忆 | 2.0 |
| 数据 | 用户画像 | 同上 | 2.0 |
| 协议 | 联邦 / 第三方协议 | 无 producer/consumer（a2a-gateway 冻结）| 2.0 启动条件：1.0 GA + 1 LLM-NPC |
| 协议 | 创作者市场 | 需要联邦基础 | 2.0+ |
| 引擎 | Saga 引擎 | 1.0 welcome 是 1 trigger + 2 step，Saga 太重 | 2.0+ |
| 引擎 | Saga DSL | 同上 | 2.0+ |
| 引擎 | BT 编辑器 | 1.0 不让用户编辑 NPC | 2.0+ |
| UI | Push 通知 | 1.0 是 Web 端在线 demo | 2.0+ |
| UI | 离线模式 | 同上 | 2.0+ |
| UI | 客户端预测 | 同上 | 2.0+ |
| UI | NPC 表情 / 头像 | 1.0 圆点 + 名字 | 2.0+ |
| 商业 | 经济系统 | 1.0 demo 无交易 | 2.0+ |
| 商业 | 合规 / GDPR | 1.0 demo 无用户数据 | 2.0+ |
| 工程 | 灾备 | 1.0 demo 单副本 | 2.0+ |
| 工程 | 压测 | 1.0 demo 单用户 | 2.0+ |
| 工程 | K8s | 1.0 demo 单机 | 2.0+ |
| 内容 | 5 NPC 全 enable | 1.0 demo 只王老板 | 2.0 |
| 内容 | NPC 寻路 / 视野 | 1.0 玩家走近 | 2.0 |
| 内容 | NPC ↔ NPC 互动 | 1.0 单 NPC | 2.0 |
| 内容 | idle 行为 | 1.0 按 schedule 走 | 2.0 |
| 内容 | 多 NPC 选择 / 群聊 | 1.0 单 NPC | 2.0 |

---

## 已知限制（1.0 demo 接受）

> 这些是 demo 中可观察到的限制，不是 bug。

### 1. First-login set 重启丢
- **现象**：重启 agent-os 后已 welcome 玩家会被再次 welcome
- **影响**：低（demo 重启是低频事件；stakeholder 演示前重启一次即可）
- **临时方案**：`docker compose restart agent-os` 清 set
- **2.0 计划**：PG 持久化 `welcomed_players(player_id, welcomed_at)`

### 2. NPCDialog 空 dialog UX
- **现象**：玩家点 NPC 但 NPC 没主动说 → dialog 弹但 options 空
- **影响**：低（提示文案"等 NPC 说话"够清晰）
- **临时方案**：等 NPC 主动触发；不要频繁点 NPC
- **2.0 计划**：点 NPC 触发 query_npc → 主动 ask 协议

### 3. demo 玩家 UUID 每次 initdb 重建都变
- **现象**：`docker compose down -v && up -d` 后 demo UUID 变
- **影响**：中（脚本硬编码 UUID 会失效）
- **临时方案**：用 SQL 动态查 UUID：`docker compose exec -T postgres psql -U aicity -d aicity -tAc "select id from player where username='demo';"`
- **2.0 计划**：保留 demo UUID 不变（initdb seed 固定 UUID）

### 4. 单副本部署
- **现象**：1.0 demo 单 compose 跑；多副本会状态不一致（玩家位置 / welcome set）
- **影响**：中（demo 不演示多副本）
- **临时方案**：单副本演示
- **2.0 计划**：多副本 + Redis 状态共享 + PG 持久化

### 5. 端口冲突
- **现象**：本地有 postgres / redis / web 占用端口时 demo 起不来
- **影响**：低（dev 环境基本不会有）
- **临时方案**：停冲突服务或改 port
- **2.0 计划**：端口可配置化

### 6. NPC 不持久化对话历史
- **现象**：每次对话独立，不存历史
- **影响**：中（玩家连续对话无上下文）
- **临时方案**：不演示"再来一次同一话题"
- **2.0 计划**：对话历史存 PG + 短期记忆 + LLM 上下文

### 7. NPC 不寻路
- **现象**：王老板按 schedule 瞬移到位置（不是平滑移动）
- **影响**：低（demo 不演示寻路）
- **临时方案**：schedule 直接更新位置
- **2.0 计划**：Sprint 11+ 的 A* 寻路（已存在 world-engine，1.0 没接）

### 8. ws-gateway 偶发断连需硬刷新
- **现象**：第二 tab 偶发不同步
- **影响**：低（硬刷新 / restart ws-gateway 即恢复）
- **临时方案**：Ctrl+Shift+R
- **2.0 计划**：自动重连 + fetchTiles 已在 Sprint 10 落实

### 9. 浏览器 cache 偶发导致 Playwright case 慢
- **现象**：第二次跑 case 比第一次慢 2-3x
- **影响**：低（首次跑 OK）
- **临时方案**：清 browser cache
- **2.0 计划**：Playwright project 加 `bypassCSP: true` + cache disable

### 10. a2a-gateway 冻结
- **现象**：a2a-gateway 6772 LOC + 74 测试通过但无 producer/consumer
- **影响**：无（不接入 compose 不影响 1.0 闭环）
- **临时方案**：CI 仍跑测，但不进 8 容器
- **2.0 计划**：2.0 启动条件之一（1 LLM-NPC + 联邦场景）

---

## Definition of Done（1.0 GA 必过）

- [ ] `acceptance_1_0` binary 跑通 5/5 步骤（exit 0）
- [ ] Playwright E2E 加 1 case 通过（点 NPC → 弹气泡 + 5 选项 + reply）
- [ ] web 端手动验证：登录 → 走 tile_0_0 → 王老板 say → 5 选项 → reply
- [ ] 录屏 3min 内能跑完 "login → walk → NPC 主动 say → 玩家回复" 全流程
- [ ] README.md 顶部 banner 声明 1.0 范围 + out-of-scope
- [ ] `docs/CHANGELOG-1.0.md` 写完（本文件）
- [ ] 8 容器 healthy + 所有单测 + 集成测通过

---

## 跨文档索引

| 主题 | 文档 |
|---|---|
| 范围 / DoD | [`1.0-ROADMAP.md`](../1.0-ROADMAP.md) |
| 演示流程 | [`1.0-demo-script.md`](../1.0-demo-script.md) |
| 5 步 binary 契约 | [`1.0-acceptance-design.md`](../1.0-acceptance-design.md) |
| Sprint 11 拆解 | [`1.0-sprint11-tasks.md`](../1.0-sprint11-tasks.md) |
| Sprint 12 拆解 | [`1.0-sprint12-tasks.md`](../1.0-sprint12-tasks.md) |
| Sprint 12 决策 | [`1.0-sprint12-decisions.md`](../1.0-sprint12-decisions.md) |
| Stakeholder 包 | [`1.0-demo-package/`](README.md) |

---

## 1.0 后续动作

### 立即（GA 后 1 周）
- [ ] stakeholder 反馈整理到 `docs/1.0-demo-feedback.md`
- [ ] 内部 beta（公司同事试用）开 30 天
- [ ] 录屏发 GitHub / 内网 / 客户群

### 短期（GA 后 1 个月）
- [ ] 2.0 范围规划（LLM 接入 / 记忆 / 联邦）
- [ ] 团队规模调整（如需）
- [ ] 工程效率评估

### 中期（GA 后 3 个月）
- [ ] 2.0 Sprint 1 启动
- [ ] 商业版本路线图
- [ ] 创作者市场预研

### 长期（2027 年）
- [ ] a2a-gateway 仍无用户 → 正式 deprecated
- [ ] 3.0 / 4.0 规划（联邦 / 创作者市场 / 商业化）
