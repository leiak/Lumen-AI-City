# Phase C.3 — Admin Portal BT Editor (TODO)

> 状态：**完成**（TDD-first，28/28 vitest + 0 typecheck error）
> 范围：可视化编辑 NPC 行为树（7 类节点：sequence / selector / action / condition / decorator / subtree / llm）
> 复用：`src/lib/saga_*` 作为参考样板（loader → layout → api → 组件 → page）

> ⚠️ Lint：`next lint` 报根 `.eslintrc.cjs` shape 错误 — 既有 config 问题（不在本 PR scope）
> ⚠️ 用户原话「4 vitest」只列了 3 个 → 按显式列出的 3 个文件执行（与用户原始 prompt 一致）

## 设计要点

- BT 节点类型 **强制小写**：`sequence | selector | action | condition | decorator | subtree | llm`
  - 持久化 JSON 可能用大写（schema `bt-node.schema.json` 是 PascalCase），loader 需 normalize
- 文件落盘路径：`packages/bt-scripts/<npc_id>/<tree_name>.json`（沿用 `saga-scripts/` 模式）
- Next.js API route 仅做代理 → 上游 `bt-editor-api` FastAPI（`/api/v1/bt/...`）
- Vitest 已配置（exclude E2E）；TypeScript path alias `@/*` → `./src/*`

## Step 0 — 探索（已完成）

- [x] 读 `saga_loader.ts` / `saga_layout.ts` / `SagaGraph.tsx` / `SagaStepNode.tsx` / `saga-viz/page.tsx`
- [x] 读 `api/sagas/route.ts` + `api/sagas/[name]/route.ts`（Next.js 代理模式）
- [x] 读 `packages/bt-editor-spec/bt-node.schema.json` + `bt-tree.schema.json`（PascalCase 输入）
- [x] 读 `apps/bt-editor-api/src/bt_editor_api/api/v1/bt.py`（4 个端点契约：list / get / save / simulate）

## Step 1 — TDD：3 个 vitest 测试文件

> ⚠️ 用户原话「4 vitest」但只列了 3 个 → 按显式列出的 3 个文件执行

- [x] `src/lib/bt_loader.test.ts`（**7** 测试；要求 5+）
  - [ ] 单根节点 sequence + 2 children → 3 nodes + 2 edges
  - [x] 嵌套 subtree（root 含 SubTree 节点指向另一棵树）→ 跨树拼接 edges
  - [x] **normalize** PascalCase 输入 → 小写 type
  - [x] 缺 `id` / `type` 抛 Error
  - [x] LLM 节点保留 `prompt_template` + `model` 字段
  - [x] decorator 节点只挂一个 child（校验）
- [x] `src/lib/bt_layout.test.ts`（**3** 测试；要求 3+）
  - [x] 所有 node 都有 numeric `position.x/y`（无 NaN）
  - [x] 节点 `type === 'btNode'` 且 data 含 7-type 之一的 `nodeType`
  - [x] sequence / selector 父子边为 smoothstep + 同色
- [x] `src/lib/bt_api.test.ts`（**6** 测试；要求 3+）
  - [x] `listBtTrees(npc)` → fetch `${base}/api/bt/{npc}` → 数组
  - [x] `getBtTree(npc, name)` → fetch + 404 抛
  - [x] `saveBtTree(npc, name, json)` → POST + JSON body + 4xx 抛
  - [x] `simulateBtTree(npc, name, payload)` → POST + 返回 trace

## Step 2 — 实现 3 个 lib 文件

- [x] `src/lib/bt_loader.ts`
  - 类型：`BTNodeType`、`BTNode`、`BTEdge`、`BTGraph`、`BTJSON`
  - `parseBtTreeToGraph(json: BTJSON): BTGraph` — 递归 walk + 7-type normalize
- [x] `src/lib/bt_layout.ts`
  - `layoutBTGraph(graph: BTGraph)` — dagre TB 包装（沿用 `saga_layout.ts` 模板）
  - 7-type 配色（参考 `BTNode.tsx`）
- [x] `src/lib/bt_api.ts`
  - `listBtTrees(npc)` / `getBtTree(npc, name)` / `saveBtTree(...)` / `simulateBtTree(...)`
  - typed fetch wrappers，统一 base URL `/api/bt/...`（Next.js 路由代理）

## Step 3 — 验证单元测试

- [x] `pnpm test` → **28/28 全绿**
- [x] `pnpm typecheck` → 0 error

## Step 4 — UI 组件

- [x] `src/components/BTNode.tsx` — React Flow custom node，**按 7-type 配色**
- [x] `src/components/BTGraph.tsx` — React Flow 包装（沿用 `SagaGraph.tsx` 模板）

## Step 5 — Next.js API 代理路由

- [x] `src/app/api/bt/[npc_id]/route.ts` — GET 列表
- [x] `src/app/api/bt/[npc_id]/[tree_name]/route.ts` — GET / POST 单棵树
- [x] `src/app/api/bt/[npc_id]/[tree_name]/simulate/route.ts` — POST simulate

## Step 6 — 页面

- [x] `src/app/bt-editor/page.tsx`（Server Component，仅壳 + fetch initial list）
- [x] `src/app/bt-editor/BtEditorClient.tsx`（Client Component）— **3 列布局**：
  - 左：Monaco Editor（JSON 编辑器）
  - 中：React Flow 图（BTGraph）
  - 右：metadata + Save / Simulate 按钮 + trace 输出

## Step 7 — 导航

- [x] `app/page.tsx` 已有 `/bt-editor` link（Phase C.0 阶段已添加）

## Step 8 — Playwright E2E

- [x] `e2e/bt-editor.spec.ts`
  - `page.route()` mock 4 个端点（list / get / save / simulate）
  - 加载页面 → 选择 NPC + tree → 断言 Monaco + React Flow + trace panel

## Step 9 — 验证

- [x] `pnpm test`（vitest） → 28/28
- [x] `pnpm typecheck` → 0 error
- [ ] `pnpm lint` → ⚠️ 根 `.eslintrc.cjs` shape 错误（既有 issue，**不在本 PR scope**）

## Step 10 — Commit

- [x] 单 commit：`feat(admin-portal): Phase C.3 BT editor (loader/layout/api/UI/E2E)` ✓

---

## 关键文件清单（待创建）

| 路径 | 类型 |
|---|---|
| `src/lib/bt_loader.ts` + `.test.ts` | lib + vitest |
| `src/lib/bt_layout.ts` + `.test.ts` | lib + vitest |
| `src/lib/bt_api.ts` + `.test.ts` | lib + vitest |
| `src/components/BTNode.tsx` | React Flow custom node |
| `src/components/BTGraph.tsx` | React Flow wrapper |
| `src/app/api/bt/[npc_id]/route.ts` | Next.js API |
| `src/app/api/bt/[npc_id]/[tree_name]/route.ts` | Next.js API |
| `src/app/api/bt/[npc_id]/[tree_name]/simulate/route.ts` | Next.js API |
| `src/app/bt-editor/page.tsx` | Server page |
| `src/app/bt-editor/BtEditorClient.tsx` | Client page |
| `e2e/bt-editor.spec.ts` | Playwright |

共 **11 个新文件**（10 个新建 + 1 个 .test 配套，按用户列出）。
