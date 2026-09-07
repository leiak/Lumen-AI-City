# AI 城邦 1.0 — Stakeholder Demo Package

> **目标**：stakeholder 拿到这个包，能在 **5 分钟内** 启动 AI 城邦 1.0 闭环，
> 看到 "玩家走近 NPC → NPC 主动打招呼 → 玩家回复 → NPC 接着聊" 的完整演示。

## 📦 包内容

| 文件 | 用途 | 阅读时长 |
|---|---|---|
| [README.md](README.md)（本文件）| 入口 + 5min 上手 banner + 目录 | 2 min |
| [quickstart.md](quickstart.md) | 5min 上手命令清单（复制粘贴）| 1 min |
| [acceptance-cheatsheet.md](acceptance-cheatsheet.md) | acceptance_1_0 命令 + 故障诊断 | 1 min |
| [recording-guide.md](recording-guide.md) | 录屏步骤 + 工具 + 后处理 | 1 min |
| [faq.md](faq.md) | 8 个 stakeholder 最常问问题 | 2 min |
| [CHANGELOG-1.0.md](CHANGELOG-1.0.md) | 1.0 release notes + 已知限制 | 3 min |

## 🎬 完整演示脚本

→ [`docs/1.0-demo-script.md`](../1.0-demo-script.md)（354 行，3min 时间轴 + Q&A + 应急）

## 🗺️ 深度文档

| 文档 | 内容 |
|---|---|
| [1.0-ROADMAP.md](../1.0-ROADMAP.md) | 范围 / DoD / 8 容器清单 / 拒绝清单 |
| [1.0-acceptance-design.md](../1.0-acceptance-design.md) | acceptance_1_0 binary 5 步接口契约 |
| [1.0-sprint11-tasks.md](../1.0-sprint11-tasks.md) | Sprint 11 任务拆解（agent-os 骨架）|
| [1.0-sprint12-tasks.md](../1.0-sprint12-tasks.md) | Sprint 12 任务拆解（玩家↔NPC 互动）|
| [1.0-sprint12-decisions.md](../1.0-sprint12-decisions.md) | Sprint 12 8 open 问题决策 |

---

## 🚀 5min 上手

```bash
# 1. 起 8 容器（30s ~ 1min 等待全部 healthy）
cd ai-city
docker compose up -d --build
docker compose ps
# 预期：postgres / redis / world-engine / api-gateway / ws-gateway / web / a2a-gateway / agent-os

# 2. 等 demo 玩家 ready（10s）
sleep 10
docker compose exec -T postgres psql -U aicity -d aicity -tAc \
  "select username from player where username='demo';"
# 预期：demo

# 3. 重启 agent-os（清 welcome set）
docker compose restart agent-os
sleep 3

# 4. 开浏览器 → http://localhost:3000/login
#    登录 demo / demo123

# 5. /city 看到 9 tile + 王老板圆点
#    走开再走回 tile_0_0 → 王老板主动 say 弹气泡

# 6. 验证（30s）
docker compose exec -T a2a-gateway /app/acceptance_1_0
# 预期：5/5 PASS, exit 0
```

详细步骤见 [quickstart.md](quickstart.md)。

---

## ❓ 出问题？

| 问题 | 看哪 |
|---|---|
| 容器起不来 | [quickstart.md §故障诊断](quickstart.md#故障诊断) |
| NPC 不主动 say | [acceptance-cheatsheet.md §故障诊断](acceptance-cheatsheet.md#故障诊断) |
| acceptance_1_0 失败 | [acceptance-cheatsheet.md §故障诊断](acceptance-cheatsheet.md#故障诊断) |
| 多 tab 不同步 | [quickstart.md §故障诊断](quickstart.md#故障诊断) |
| Stakeholder 提问 | [faq.md](faq.md) |

---

## 🎥 录屏

录屏步骤见 [recording-guide.md](recording-guide.md)。
录完的 webm 放 `docs/assets/demo-1.0.webm`（≤ 5MB，README 可嵌）。

---

## 📋 1.0 范围 / 不在范围

**范围（MUST）**：
- 8 容器：postgres / redis / world-engine / api-gateway / ws-gateway / web / a2a-gateway / agent-os
- 1 个玩家走 3×3 城邦
- 1 个 NPC（王老板）主动打招呼 + 5 选项对话
- 1 条剧本（新玩家入场 welcome）
- 多 tab 实时同步
- 5 步自动化验收（acceptance_1_0）

**不在范围（明确切割，2.0+ 才做）**：
- LLM / AI 生成（1.0 用预设对话树）
- 记忆 / Milvus / 用户画像
- 联邦 / 创作者市场 / 第三方协议
- BT 编辑器 / Saga DSL / Saga 引擎
- Push 通知 / 离线 / 客户端预测
- 经济 / 合规 / 灾备 / 压测 / K8s

详细边界见 [1.0-ROADMAP.md §二](../1.0-ROADMAP.md)。

---

## 🎯 成功判据

stakeholder 走出演示时能向同事复述这 3 件事：

1. **"玩家走进城邦，NPC 会主动打招呼。"**（NPC 主动 say，1.0 核心）
2. **"玩家可以选话题，NPC 接着聊。"**（多轮对话，行为树）
3. **"5 步自动化验收都过了。"**（工程可信度）

如果只能记 1 件 → **NPC 主动 say**（1.0 最差异化）。

---

## 📞 联系

- **1.0 范围问题** → 看 `1.0-ROADMAP.md`
- **技术细节** → 看 `1.0-acceptance-design.md`
- **任务状态** → 看 `1.0-sprint11-tasks.md` / `1.0-sprint12-tasks.md`
- **实施决策** → 看 `1.0-sprint12-decisions.md`
- **演示流程** → 看 [`docs/1.0-demo-script.md`](../1.0-demo-script.md)
- **反馈 / 改进建议** → 找 Tech Lead
