# 行为树可视化编辑器 PRD（v1.0）

> **代号**：BT-Editor / 简称 `bte`
> **模块归属**：[§C.5 创作者模式](11-技术细节与玩法模式.md) 的核心组件，对应 [§E.1 行为树编辑器](11-技术细节与玩法模式.md) 的工程实现
> **状态**：Draft（待评审）
> **目标上线**：MVP+2（即阶段 B，单城稳定版完成后）

---

## 0. 文档信息

| 字段 | 值 |
|---|---|
| 版本 | v1.0 |
| 作者 | 产品 + Agent OS 负责人（待指定） |
| 创建日期 | 2026-09-03 |
| 关联文档 | [§E.1 行为树编辑器](11-技术细节与玩法模式.md) / [§19.10 行为树](05-Agent-OS.md) / [§02 NPC 人设](02-NPC人设与剧本.md) / [§23.5 社区策略](07-MVP与ADR.md) |
| 评审节点 | 产品 / 前端 / 后端 / 测试 / 法务（创作者版权）四方评审 |

---

## 1. 概述

### 1.1 背景

AI 城邦要让 NPC 像真人一样生活、决策与社交，但完全靠 LLM 决策存在两个问题：
1. **成本**：1 万 NPC 全部走 LLM 每天约 $8,640（§49.6 实测）。
2. **可控性**：纯 LLM 难以让内容创作者精确表达"NPC 在早 6-10 点应该做什么"。

行为树（Behavior Tree）是公认的"AI 决策可表达性 + 低成本"的最佳载体：90% 的日常动作由确定性行为树执行，10% 的复杂分支才升级到 LLM（§19.10 / §B.3）。

但目前创作者需要**手写 JSON** 才能编辑行为树，门槛高、错误多、可视化差。本 PRD 旨在交付一款**拖拽式、零代码、强验证、可调试**的可视化编辑器。

### 1.2 问题陈述

| 谁 | 当前痛点 | 影响 |
|---|---|---|
| 同人创作者 Lily | 不会写代码，想让 NPC 按特定规则行动 | 放弃创作 |
| 剧情策划 陈 | 想用熟悉的"节点编辑器"工作 | 需手动写 JSON，效率低 |
| 内容审核员 王 | 难以审查"决策树是否合理" | 审核耗时、易漏 |
| 玩家 张 | 想"调教"自家 NPC 让它做事 | 无工具 |

### 1.3 目标用户

- **首要目标**：让**非工程师创作者** 30 分钟内完成首个 BT。
- **次要目标**：让**剧情策划** 高效迭代节日流程。
- **第三目标**：让**专业开发者** 调试自家 NPC 时可视化追踪。

---

## 2. 目标与非目标

### 2.1 Goals（v1.0 必做）

| ID | 目标 | 度量 |
|---|---|---|
| G1 | 创作者 30 分钟内完成首个 BT（含保存 / 验证） | 新手任务时长 |
| G2 | BT 编辑不需写代码 | 完成率 = 100% |
| G3 | 实时验证错误反馈 < 500ms | 实测延迟 |
| G4 | 沙箱 24h 模拟可一键启动 | 沙箱报告呈现 |
| G5 | 与 §E.1 行为树运行时无缝集成 | BT 编译产物可被 agent-os 加载 |
| G6 | 编辑器性能流畅（100 节点 60fps） | 渲染帧率 |
| G7 | 与 §23.6 开源策略一致：核心可被社区复刻 | BT JSON Schema 开源 |

### 2.2 Non-Goals（v1.0 不做）

- ❌ 多人协作编辑（v2.x 加入，CRDT + Yjs）
- ❌ 移动端编辑器（移动端只读、不编辑）
- ❌ AI 自动生成 BT（保留给 v2，依赖 §E.2 Saga DSL）
- ❌ 跨 BT 复用（SubTree 节点存在，但 marketplace 在 v2.x）
- ❌ 3D 节点编辑器（仅 2D）
- ❌ 完整 NPC 配置编辑（仅负责 BT，其他在 NPC 模板编辑器）

---

## 3. 用户画像（Personas）

### P1：路人创作者 Lily

- **背景**：Minecraft 老玩家，玩过 Roblox，零编程经验。
- **动机**：看到有人做了"AI 老李"想自己也做一个。
- **使用频率**：1-2 次 / 周，每次 1-2h。
- **设备**：笔记本电脑（Chrome），偶尔手机看文档。
- **痛点**：看不懂 JSON 报错信息；不知道哪些节点类型可用。

### P2：剧情策划 陈

- **背景**：游戏公司 5 年剧情策划，会用节点编辑器（如 RPG Maker）。
- **动机**：节日活动需要"老李如何筹备元旦"。
- **使用频率**：每月 2-4 次，每次 4-8h。
- **设备**：双屏工作站。
- **痛点**：跨 NPC 复用行为时复制粘贴繁琐。

### P3：AI 工程师 张

- **背景**：AI 公司工程师，会 Python，正在把自家 NPC 接入城邦。
- **动机**：调试 NPC 决策链路。
- **使用频率**：每天，每次 0.5-2h。
- **设备**：高配工作站，多显示器。
- **痛点**：看不到决策为什么走 A 而不是 B 分支。

### P4：内容审核员 王

- **背景**：法务 + 心理双背景，负责审查创作者发布的 NPC。
- **动机**：确保 NPC 不会输出违规内容。
- **使用频率**：每天 8h，每次审 20-30 个 BT。
- **设备**：标准办公电脑。
- **痛点**：现有审核基于规则，没有"决策可视化"工具。

---

## 4. 用户故事（User Stories）

### 4.1 Lily 的故事

```
US-001：作为路人创作者，我希望
  Given 我刚注册并进入编辑器
  When 我点击"新建 NPC 行为树"
  Then 系统为我创建一个空画布 + 教程引导
   And 我能在 5 分钟内拖出"NPC 早上 7 点打招呼"的简单 BT
```

```
US-002：作为路人创作者，我希望
  Given 我正在编辑 BT
  When 我做出一个错误（Selector 直接挂 Action）
  Then 编辑器立即给出红色警告 + "建议改用 Sequence"
   And 我点警告可直接跳到错误节点
```

```
US-003：作为路人创作者，我希望
  Given 我编辑完了 BT
  When 我点"试运行"
  Then 系统启动 24h 模拟并在 1 分钟内返回"模拟报告"
   And 报告告诉我"NPC 在 24h 内做了 120 个动作，其中 95% 是打招呼"
```

### 4.2 陈的故事

```
US-010：作为剧情策划，我希望
  Given 我在做"元旦灯会"流程
  When 我需要"老李在 17:00 准备摊位"
  Then 我能拖一个"时间条件 + 动作"组合实现
   And 能用模板（"时间事件"）一键插入
```

```
US-011：作为剧情策划，我希望
  Given 我有"老李"和"阿忆"两个 BT
  When 我想共用"问候动作"
  Then 我能用 SubTree 引用，让两棵树共享一个子树
```

### 4.3 张的故事

```
US-020：作为 AI 工程师，我希望
  Given 我在调试 NPC 决策
  When 我设置断点"老李说这句话"
  Then 真实运行时 NPC 触发此句时暂停 + 弹出决策上下文
   And 我能修改参数后让 NPC 继续
```

```
US-021：作为 AI 工程师，我希望
  Given 我在复现"NPC 发疯"问题
  When 我打开决策回放
  Then 系统按时间序列展示"哪些节点被走过 / 哪些 LLM 调用"
```

### 4.4 王的故事

```
US-030：作为内容审核员，我希望
  Given 我要审一个创作者的 BT
  When 我点开审核视图
  Then 我能"单步走完整棵 BT"看每个 Action 是否触达红线
   And 违规节点被自动高亮
```

---

## 5. 功能需求（Functional Requirements）

### 5.1 FR-1：节点系统（核心数据模型）

| ID | 节点类型 | 参数 | 用途 |
|---|---|---|---|
| FR-1.1 | **Sequence** | `children[]` | 子节点全成功才成功 |
| FR-1.2 | **Selector** | `children[]` | 子节点首个成功即返回 |
| FR-1.3 | **Condition** | `expression: string` | 返回 true/false |
| FR-1.4 | **Action** | `verb, params, outputs[]` | 执行具体动作 |
| FR-1.5 | **Decorator** | `Loop/Inverter/UntilFail/Retry` | 包装子节点 |
| FR-1.6 | **SubTree** | `bt_ref: string` | 引用另一棵 BT |
| FR-1.7 | **LLM** | `prompt_template, model, fallback` | 调 LLM 输出 action |

#### 节点 Schema 示例

```json
{
  "bt_id":         "bt_lao_li_morning_v2",
  "version":       "1.4",
  "schema":        "bte/v1",
  "root_node_id":  "n_root",
  "nodes": {
    "n_root": {
      "type":        "Selector",
      "description": "早上开店流程",
      "children":    ["n_seq_morning", "n_seq_afternoon", "n_default"]
    },
    "n_seq_morning": {
      "type":     "Sequence",
      "children": ["n_cond_is_morning", "n_cond_shop_open", "n_act_greet"]
    },
    "n_cond_is_morning": {
      "type":       "Condition",
      "expression": "context.hour >= 6 && context.hour < 10"
    },
    "n_act_greet": {
      "type":    "Action",
      "verb":    "npc.greet",
      "params":  { "target": "{{visible_agent}}", "tone": "warm" },
      "outputs": ["speech"]
    }
  }
}
```

### 5.2 FR-2：画布与拖拽

| ID | 功能 | 详细说明 |
|---|---|---|
| FR-2.1 | 节点调色板 | 左侧 7 类节点卡片，可拖入画布 |
| FR-2.2 | 拖拽连线 | 子节点拖到父节点上自动连线 |
| FR-2.3 | 框选 + 移动 | 支持 Shift 多选 + 拖动整组 |
| FR-2.4 | 撤销/重做 | Undo/Redo 无限步（本地内存 + 服务端 ops log） |
| FR-2.5 | 缩放/平移 | 鼠标滚轮缩放，右键拖动平移 |
| FR-2.6 | 节点搜索 | 按 `id` 或 `description` 搜索并定位 |
| FR-2.7 | 复制/粘贴 | Ctrl+C/Ctrl+V，整组节点支持 |
| FR-2.8 | 自动布局 | 一键按层次结构整理（`dagre` 算法） |
| FR-2.9 | 小地图 | 右下角缩略图，可拖动定位 |
| FR-2.10 | 触摸支持 | 平板设备可拖拽（移动端仅浏览） |

### 5.3 FR-3：实时验证

| 类型 | 检查项 | 反馈 |
|---|---|---|
| **结构性** | DAG（无环）、根节点存在、所有节点引用合法、连线无悬空 | 红色框 + 工具栏图标 |
| **语义性** | Selector 不能直接是叶子、Sequence 至少 2 子节点、Action 引用合法动作库、Condition 表达式可解析 | 黄色警告 + 修正建议 |
| **可达性** | 通过 BFS 检查孤儿节点（warn） | 灰色半透明 |

#### 验证延迟要求

| 操作 | 触发 | 反馈延迟 |
|---|---|---|
| 拖动节点 | onDrop | < 100ms |
| 修改参数 | onChange | < 200ms |
| 删除节点 | onDelete | < 100ms |
| 保存 | onSave | 完整异步验证 < 500ms |

### 5.4 FR-4：沙箱测试（24h 模拟）

| ID | 功能 | 详细说明 |
|---|---|---|
| FR-4.1 | 一键启动 | 按钮"试运行" → 提交到 sandbox-runner |
| FR-4.2 | 加速模拟 | 24h 实际 → 1 分钟模拟（1440x 加速） |
| FR-4.3 | 报告输出 | actions_taken / unique_actions / llm_calls / avg_latency_ms / divergent_actions |
| FR-4.4 | 报告分享 | URL 可分享给队友查看 |
| FR-4.5 | 多次对比 | 同 BT 改参数前后对比（diff view） |
| FR-4.6 | 真实世界映射 | 可选"使用玩家最常出现场景"作为初始环境 |

### 5.5 FR-5：版本管理

| ID | 功能 | 详细说明 |
|---|---|---|
| FR-5.1 | 自动保存 | 编辑时每 30s 自动保存草稿（服务端） |
| FR-5.2 | 版本快照 | 每次发布创建一个不可变版本 |
| FR-5.3 | 版本列表 | `/versions` 列出所有版本，可对比 |
| FR-5.4 | 版本回滚 | 一键回到历史版本 |
| FR-5.5 | Fork | 任何人可 fork 任意公开 BT（在原作者基础上修改） |
| FR-5.6 | Diff 视图 | 两个版本差异可视化（节点 / 连线 / 参数） |

### 5.6 FR-6：调试器

| ID | 功能 | 详细说明 |
|---|---|---|
| FR-6.1 | 单步 | 点"下一步"走完一个节点 |
| FR-6.2 | 断点 | 在任意节点右键"添加断点" |
| FR-6.3 | 实时回放 | 选择某 trace_id → 时间轴可视化 |
| FR-6.4 | What-if | 在某节点暂停后注入"假设"参数 |
| FR-6.5 | 上下文查看 | 节点触发时的 context（如 `visible_agents` 内容） |
| FR-6.6 | 性能火焰图 | 显示哪个节点耗时最长 |

### 5.7 FR-7：LLM 节点 Prompt 模板编辑器

| ID | 功能 | 详细说明 |
|---|---|---|
| FR-7.1 | Monaco 编辑器 | LLM 节点内嵌 Monaco，写 prompt |
| FR-7.2 | 变量插入 | `{{npc.name}}` 等变量自动提示 |
| FR-7.3 | 模板预览 | 实际渲染 prompt（替换变量）实时看 |
| FR-7.4 | 试调 | 用一个 NPC 试调 LLM，3 秒看到返回 |
| FR-7.5 | Safe-LLM 校验 | 保存过 §28 Schema + Safe-LLM 拦截恶意 prompt |

### 5.8 FR-8：市场（Marketplace）

| ID | 功能 | 详细说明 |
|---|---|---|
| FR-8.1 | 公开 BT 列表 | 按热度 / 时间 / 评分排序 |
| FR-8.2 | 详情页 | BT 可视化预览 + 报告 + 评论 |
| FR-8.3 | 评分 / 评论 | 5 星 + 评论（与 §23 商业模式联动） |
| FR-8.4 | Fork 计数 | 显示 fork 数 + 衍生作品 |
| FR-8.5 | 标签搜索 | tag 分类（如"节日"、"日常"、"对抗"） |
| FR-8.6 | 内容审核 | 上架前必须过 §28 + §36 双层审核 |

### 5.9 FR-9：协作编辑（v1.0 简化版）

| ID | 功能 | 详细说明 |
|---|---|---|
| FR-9.1 | 单人编辑 | v1.0 仅支持单人（锁机制） |
| FR-9.2 | 编辑锁 | 当前编辑者独占 30 分钟 |
| FR-9.3 | 旁观模式 | 其他用户可读 + 评论 |
| FR-9.4 | 多人实时 | v2.x 加入（CRDT + Yjs） |

### 5.10 FR-10：导入导出

| ID | 功能 | 详细说明 |
|---|---|---|
| FR-10.1 | JSON 导出 | BT JSON Schema 标准格式 |
| FR-10.2 | JSON 导入 | 支持粘贴或上传 |
| FR-10.3 | 图片导出 | PNG / SVG 节点图（便于贴 Notion） |
| FR-10.4 | 决策视频导出 | 沙箱报告录屏为 mp4 |

---

## 6. 非功能需求（NFR）

### 6.1 性能

| 指标 | 要求 | 实测方法 |
|---|---|---|
| 节点编辑响应 | 拖动 / 删除 < 100ms | Lighthouse + chrome devtools |
| 100 节点画布帧率 | ≥ 60fps | React Flow Profiler |
| 1000 节点画布帧率 | ≥ 30fps | 同上 |
| 服务端编译 | < 50ms | 单元测试 |
| 沙箱启动 | < 5s | 沙箱报告 timeout |
| 沙箱报告 | < 60s（24h 模拟） | 沙箱报告生成时长 |

### 6.2 可用性

- 99.5% SLA
- P95 响应 < 200ms
- 部署 3 个 region 容灾

### 6.3 兼容性

- 浏览器：Chrome 100+、Firefox 100+、Safari 15+、Edge 100+
- 不支持：IE、旧版 Edge、移动端编辑

### 6.4 安全

- 所有 LLM prompt 过 Safe-LLM（§28）
- 所有 Action verb 在合法白名单内（§18 接口清单）
- BT 不可绕过 §36 红线审核
- 创作者 fork 不暴露原作者私钥

### 6.5 国际化

- v1.0 支持中文 + 英文
- 文案走 i18n key
- BT 节点 description 支持任意语言

### 6.6 可维护性

- BT JSON Schema 公开（CC BY 4.0）
- 前端组件库统一（Ant Design / shadcn）
- 单元测试覆盖率 > 80%
- E2E 测试覆盖所有用户故事

---

## 7. UX 关键流程

### 7.1 主流程：创建 NPC 行为树

```
1. 创作者点击"新建 BT"
2. 选择 NPC 模板（或从空白开始）
3. 进入编辑器（空白 + 教程浮窗）
4. 拖入根 Selector
5. 添加子节点（Sequence + Action）
6. 编辑 Action 参数
7. 实时验证：红色警告 + 修正
8. 试运行：24h 模拟报告
9. 发布到市场（或仅自己用）
```

### 7.2 异常流程

| 异常 | 处理 |
|---|---|
| 节点循环引用 | 拖拽时阻止 + 提示"已存在路径" |
| Action verb 不存在 | 实时警告 + 提供"建议 verb"列表 |
| 沙箱超时 | 5min 后返回"沙箱繁忙，请稍后" |
| 版本冲突 | 提示"他人正在编辑" + 自动跳转版本对比 |
| Safe-LLM 拦截 prompt | 编辑器红色警告 + 提供"合规 prompt 模板" |

---

## 8. 技术架构

### 8.1 整体架构图

```
┌─────────────────────┐
│  React Flow Frontend │  ← bte-web (Next.js)
│  + Monaco (Prompt)  │
└──────────┬──────────┘
           │ REST + WebSocket
┌──────────▼──────────┐
│   bte-gateway       │  ← Go：路由、限流、鉴权
└──────────┬──────────┘
           │ gRPC
┌──────────▼──────────────────────────────────┐
│  bte-services (Python)                       │
│  ┌──────────────┐ ┌──────────────┐          │
│  │ bte-editor   │ │ bte-compiler │          │
│  │ (CRUD ops)   │ │ (BT JSON→fn) │          │
│  └──────────────┘ └──────────────┘          │
│  ┌──────────────┐ ┌──────────────┐          │
│  │ bte-validator│ │ bte-sandbox  │          │
│  │ (3 类验证)   │ │ (24h 模拟)   │          │
│  └──────────────┘ └──────────────┘          │
└──────────┬──────────────────────────────────┘
           │
┌──────────▼──────────┐
│   agent-os 加载     │  ← 编译后的 BT 函数被 Agent OS 加载
└─────────────────────┘
```

### 8.2 前端技术栈

| 层 | 选型 |
|---|---|
| 框架 | Next.js 15 + React 19 |
| 画布 | React Flow 11 |
| Prompt 编辑 | Monaco Editor |
| 状态管理 | Zustand |
| 实时同步（v2.x） | Yjs + WebSocket |
| UI 组件 | shadcn/ui + Radix |
| 样式 | Tailwind CSS |
| 动画 | Framer Motion |
| 测试 | Vitest + Playwright |

### 8.3 后端技术栈

| 层 | 选型 |
|---|---|
| 服务 | Python 3.12 + FastAPI |
| 验证引擎 | Python AST + 自研 DSL 解析器 |
| 沙箱 | Docker container（隔离 namespace + 资源限额） |
| LLM | Anthropic SDK + LiteLLM |
| DB | PostgreSQL（BT 持久化）+ Redis（autosave）+ S3（沙箱报告） |
| 部署 | K8s + KEDA |

### 8.4 关键服务职责

| 服务 | 职责 |
|---|---|
| **bte-gateway** | 路由 + 限流 + 鉴权 |
| **bte-editor** | CRUD BT（保存、加载、fork、版本管理） |
| **bte-validator** | 3 类实时验证（结构 / 语义 / 可达） |
| **bte-compiler** | BT JSON → 可执行 Python 函数（§E.1 服务端编译器） |
| **bte-sandbox** | 沙箱 24h 模拟 + 报告生成 |
| **bte-debugger** | 单步 / 断点 / 回放 / what-if |
| **bte-marketplace** | 公开 BT 浏览 + 评论 + 评分 |

---

## 9. 数据模型详细

### 9.1 BT 主表（PostgreSQL）

```sql
CREATE TABLE behavior_trees (
    bt_id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    bt_key           VARCHAR(64) UNIQUE NOT NULL,    -- e.g. "bt_lao_li_morning"
    version          VARCHAR(16) NOT NULL,           -- 语义版本 e.g. "1.4"
    schema           VARCHAR(16) NOT NULL DEFAULT 'bte/v1',
    name             VARCHAR(128) NOT NULL,
    description      TEXT,
    author_id        UUID NOT NULL REFERENCES users(id),
    npc_template_id  VARCHAR(64),                    -- 关联 NPC 模板（可空）
    root_node_id     VARCHAR(64) NOT NULL,
    status           VARCHAR(16) NOT NULL DEFAULT 'draft',  -- draft|published|deprecated|under_review
    nodes_json       JSONB NOT NULL,                 -- 完整 BT JSON
    metadata         JSONB NOT NULL DEFAULT '{}',     -- {tags, category, license}
    fork_of          UUID REFERENCES behavior_trees(bt_id),
    forked_count     INTEGER DEFAULT 0,
    download_count   INTEGER DEFAULT 0,
    rating_avg       FLOAT DEFAULT 0,
    rating_count     INTEGER DEFAULT 0,
    created_at       TIMESTAMPTZ DEFAULT NOW(),
    updated_at       TIMESTAMPTZ DEFAULT NOW(),
    published_at     TIMESTAMPTZ,
    CHECK (status IN ('draft','published','deprecated','under_review'))
);
CREATE INDEX idx_bt_author      ON behavior_trees(author_id);
CREATE INDEX idx_bt_status      ON behavior_trees(status);
CREATE INDEX idx_bt_template    ON behavior_trees(npc_template_id);
CREATE INDEX idx_bt_created     ON behavior_trees(created_at DESC);
CREATE INDEX idx_bt_rating      ON behavior_trees(rating_avg DESC)
    WHERE status = 'published';
CREATE INDEX idx_bt_metadata    ON behavior_trees USING GIN (metadata jsonb_path_ops);
```

### 9.2 版本快照表

```sql
CREATE TABLE bt_versions (
    version_id       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    bt_id            UUID REFERENCES behavior_trees(bt_id) ON DELETE CASCADE,
    version          VARCHAR(16) NOT NULL,
    nodes_json       JSONB NOT NULL,                 -- 不可变快照
    changelog        TEXT,
    created_by       UUID REFERENCES users(id),
    created_at       TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(bt_id, version)
);
```

### 9.3 编辑锁表

```sql
CREATE TABLE bt_edit_locks (
    bt_id            UUID PRIMARY KEY REFERENCES behavior_trees(bt_id),
    locked_by        UUID NOT NULL REFERENCES users(id),
    acquired_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at       TIMESTAMPTZ NOT NULL,            -- acquired_at + 30min
    heartbeat_at     TIMESTAMPTZ DEFAULT NOW()
);
```

### 9.4 沙箱报告表

```sql
CREATE TABLE bt_sandbox_reports (
    report_id        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    bt_id            UUID REFERENCES behavior_trees(bt_id),
    bt_version       VARCHAR(16),
    npc_template_id  VARCHAR(64),
    actions_taken    INTEGER,
    unique_actions   INTEGER,
    llm_calls        INTEGER,
    avg_latency_ms   FLOAT,
    divergent        JSONB,
    params           JSONB,
    created_at       TIMESTAMPTZ DEFAULT NOW()
);
```

### 9.5 评论与评分

```sql
CREATE TABLE bt_ratings (
    rating_id        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    bt_id            UUID REFERENCES behavior_trees(bt_id) ON DELETE CASCADE,
    user_id          UUID REFERENCES users(id),
    stars            SMALLINT NOT NULL CHECK (stars BETWEEN 1 AND 5),
    comment          TEXT,
    created_at       TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(bt_id, user_id)
);
```

---

## 10. API 规范（RESTful）

| Method | Path | 描述 | 鉴权 |
|---|---|---|---|
| `POST` | `/api/v1/creator/bt` | 创建 BT（返回空 BT + bt_id） | 是 |
| `GET` | `/api/v1/creator/bt/:id` | 读取 BT（含 nodes_json） | 是 |
| `PUT` | `/api/v1/creator/bt/:id` | 更新 BT（自动保存 + 手动保存） | 是 |
| `DELETE` | `/api/v1/creator/bt/:id` | 删除 BT（软删，30 天回收） | 是 |
| `POST` | `/api/v1/creator/bt/:id/validate` | 触发完整验证 | 是 |
| `POST` | `/api/v1/creator/bt/:id/compile` | 编译 BT → 可执行函数（预热） | 是 |
| `POST` | `/api/v1/creator/bt/:id/sandbox/run` | 启动沙箱 24h 模拟 | 是 |
| `GET` | `/api/v1/creator/bt/:id/sandbox/reports` | 列出历史沙箱报告 | 是 |
| `GET` | `/api/v1/creator/bt/:id/sandbox/reports/:report_id` | 读取报告详情 | 是 |
| `POST` | `/api/v1/creator/bt/:id/publish` | 发布到市场（触发审核） | 是 |
| `GET` | `/api/v1/creator/bt/:id/versions` | 列出所有版本 | 是 |
| `POST` | `/api/v1/creator/bt/:id/fork` | Fork 一份到我的空间 | 是 |
| `POST` | `/api/v1/creator/bt/:id/lock` | 获取编辑锁 | 是 |
| `POST` | `/api/v1/creator/bt/:id/unlock` | 释放编辑锁 | 是 |
| `POST` | `/api/v1/creator/bt/:id/debug` | 启动调试会话 | 是 |
| `GET` | `/api/v1/creator/bt/marketplace?tags=&sort=` | 浏览市场 | 是 |
| `POST` | `/api/v1/creator/bt/:id/rate` | 评分 | 是 |
| `POST` | `/api/v1/creator/bt/:id/comment` | 评论 | 是 |

**统一响应信封**：与 §18.6 一致（`code, msg, data, trace_id, server_ts`）。

**错误码扩展**（与 §18.5 一致，新增）：
```
BT_001  BT 不存在
BT_002  BT 已被锁
BT_003  编辑冲突
BT_004  验证失败
BT_005  编译失败
BT_006  沙箱繁忙
BT_007  沙箱超时
BT_008  审核未通过
BT_009  无发布权限
```

---

## 11. 验收标准（Definition of Done）

### 11.1 功能验收

- [ ] G1: 5 名非工程师 30 分钟内完成首个 BT（实测）
- [ ] G2: 0 行代码完成 BT 编辑
- [ ] G3: 验证错误反馈 < 500ms（实测）
- [ ] G4: 沙箱报告 60s 内返回
- [ ] G5: 编译产物可被 agent-os 加载（实测）
- [ ] G6: 100 节点画布 60fps（实测）
- [ ] G7: BT JSON Schema 公开

### 11.2 质量验收

- [ ] 单元测试覆盖率 > 80%
- [ ] E2E 测试覆盖所有用户故事
- [ ] 性能：100 节点拖动响应 < 100ms
- [ ] 安全：恶意 BT 拦截 100%
- [ ] 安全：恶意 prompt 拦截 100%（§28）
- [ ] 可用性：SLA 99.5%
- [ ] 文档：用户手册 + 开发者文档完整

### 11.3 内容验收

- [ ] 教程引导：5 个模板（"开店流程"、"节日活动"、"问路"、"夜晚巡逻"、"对话树"）
- [ ] 内置 50 个常用 verb（npc.greet, npc.walk, npc.say, npc.think, ...）
- [ ] 内置 30 个 Condition 表达式（time, weather, relationship, has_item, ...）

---

## 12. 成功指标（KPIs）

### 12.1 创作者侧

| 指标 | 目标 | 测量 |
|---|---|---|
| **新创作者 30 天激活率** | > 40% | 创建 1 个 BT 并完成试运行 |
| **平均编辑时长 / BT** | < 2h | 创建到首次发布 |
| **BT 发布成功率** | > 60% | 沙箱通过 / 提交发布 |
| **市场评分** | > 4.2 / 5 | 所有已发布 BT 的平均分 |
| **Fork 数 / BT** | > 5 | 中位数 |

### 12.2 玩家侧

| 指标 | 目标 | 测量 |
|---|---|---|
| **创作者 NPC 在玩家中使用率** | > 30% | 玩家使用 NPC 中创作者贡献比例 |
| **玩家对创作者 NPC 好感** | > 4.0 / 5 | NPS 调查 |
| **创作者收入中位数** | > $500 / 月 | top 30% 创作者 |

### 12.3 平台侧

| 指标 | 目标 | 测量 |
|---|---|---|
| **BT 总数** | > 10,000（上线 6 月内） | DB count |
| **每日 BT 操作** | > 1M | 决策日志 |
| **LLM 调用占比** | < 30% | BT 路径 vs LLM 路径 |
| **审核违规率** | < 1% | 上架后被下架的 BT 比例 |

---

## 13. 里程碑（Milestones）

### M1：基础编辑（4 周）

| 周 | 任务 |
|---|---|
| W1 | 节点调色板 + 拖拽 + 画布 |
| W2 | 节点 CRUD + 撤销/重做 |
| W3 | 自动布局 + 小地图 + 搜索 |
| W4 | 自动保存 + 草稿持久化 |

**交付**：可拖拽、可保存的 BT 编辑器（MVP）。

### M2：验证 + 沙箱（3 周）

| 周 | 任务 |
|---|---|
| W5 | 结构性 + 语义性 + 可达性验证 |
| W6 | 沙箱 24h 模拟 + 报告生成 |
| W7 | 沙箱报告 UI + 分享 |

**交付**：可验证、可试运行的编辑器。

### M3：编译 + Prompt（3 周）

| 周 | 任务 |
|---|---|
| W8 | bte-compiler 服务 + 与 agent-os 集成 |
| W9 | LLM 节点 + Monaco prompt 编辑 |
| W10 | Safe-LLM 校验 + 模板 |

**交付**：可运行的 BT + LLM 节点。

### M4：市场 + 协作基础（2 周）

| 周 | 任务 |
|---|---|
| W11 | 市场页面 + 评分 + 评论 |
| W12 | Fork + 版本管理 + 编辑锁 |

**交付**：可发布、可分享的编辑器。

### M5：调试器（2 周）

| 周 | 任务 |
|---|---|
| W13 | 单步 + 断点 + 上下文 |
| W14 | what-if + 性能火焰图 |

**交付**：可调试的编辑器（v1.0 完整版）。

---

## 14. 风险与缓解

| 风险 | 概率 | 影响 | 缓解措施 |
|---|---|---|---|
| **LLM Prompt 失控** | 中 | 高 | Monaco 内嵌 Safe-LLM 拦截 + 模板化（ |
| **大型 BT 性能（>500 节点）** | 中 | 中 | 虚拟化渲染 + 缩略图 + 分页 |
| **创作者抄袭** | 中 | 中 | Fork 关系图谱溯源 + 内容指纹 |
| **沙箱被滥用** | 中 | 高 | 配额（每人 10 次/天）+ 资源限额 |
| **BT 编译错误难定位** | 中 | 中 | 错误精确到节点 + 行号 + 修复建议 |
| **协作冲突（v1.0 单人限制）** | 低 | 中 | 编辑锁 + 30min 超时 + 提示 |
| **审核员负担过重** | 中 | 中 | LLM 预审 + 抽审 + 风险分级 |
| **创作者激励不足** | 中 | 中 | 与 §23 商业模式挂钩 + 排行榜 |

---

## 15. 待决策问题（Open Questions）

| ID | 问题 | 状态 |
|---|---|---|
| Q1 | LLM 节点是否允许"自定义 Function Call"？ | 待评审 |
| Q2 | BT 是否支持"运行时热更新"（不重启 agent）？ | 待评审 |
| Q3 | Marketplace 是否接入真实货币结算？v1.0 是否只发积分？ | 待评审 |
| Q4 | 创作者是否可上传"prompt 模板"为付费产品？ | 待评审 |
| Q5 | BT 是否支持"环境变量"（如不同城市配置）？ | 待评审 |
| Q6 | 编辑器是否提供"AI 辅助"按钮（一句话生成节点）？ | 待评审 |
| Q7 | 与 §E.2 Saga DSL 是否合并为统一 DSL？ | 待评审 |

---

## 16. 附录

### 16.1 竞品对比

| 竞品 | 优点 | 我们的差异 |
|---|---|---|
| **Blueprints (UE)** | 节点丰富、社区成熟 | 不支持 LLM 节点、不支持沙箱 |
| **Behavior Designer** | Unity 集成深 | 同上 |
| **Node-RED** | 流程编排、IoT | 不是 BT 语义 |
| **ROS BT** | 机器人场景成熟 | 与 NPC 域差距大 |
| **Scratch** | 教育友好 | 不是 BT，只是积木 |

**结论**：市场上无"AI 域 + BT + LLM + 创作者"一体化编辑器，本产品差异化强。

### 16.2 术语表

| 术语 | 含义 |
|---|---|
| BT | Behavior Tree，行为树 |
| Selector | 选择节点 |
| Sequence | 顺序节点 |
| Condition | 条件节点 |
| Action | 动作节点 |
| Decorator | 装饰节点 |
| SubTree | 子树引用 |
| LLM | 大语言模型节点 |
| Verb | Action 的执行动词 |
| Fork | 派生副本 |
| Sandbox | 沙箱环境 |
| Trace | 决策链路 |
| DAG | 有向无环图 |

### 16.3 引用文档

| 引用 | 用途 |
|---|---|
| [§19.10 行为树](05-Agent-OS.md) | 运行时行为树执行 |
| [§E.1 行为树编辑器](11-技术细节与玩法模式.md) | 原始设计 |
| [§02 NPC 人设](02-NPC人设与剧本.md) | NPC 模板与 BT 关联 |
| [§18 API 设计](04-API设计.md) | 通用 API 规范 |
| [§28 A2A 安全](08-架构优化v1.md) | LLM prompt 安全校验 |
| [§23 商业模式](07-MVP与ADR.md) | 创作者版税机制 |
| [§20 A2A 协议](06-A2A协议.md) | 第三方 Agent 接入 |

---

## 17. 评审签到

| 角色 | 姓名 | 日期 | 签字 |
|---|---|---|---|
| 产品负责人 | — | — | — |
| 前端负责人 | — | — | — |
| 后端负责人 | — | — | — |
| Agent OS 负责人 | — | — | — |
| 测试负责人 | — | — | — |
| 法务（创作者版权） | — | — | — |
| 安全负责人 | — | — | — |

---

> **下一步**：把本 PRD 走完四方评审后冻结 v1.0，进入 M1（基础编辑，4 周）开发。
> **关联 issue**：待创建于 [project/bt-editor](待定)
> **相关 ADR**：待创建 ADR-NNN：BT 编辑器选型与技术栈（待评审）

**文档结束**。