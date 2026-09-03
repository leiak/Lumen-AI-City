# Contributing to AI City

感谢你对 AI 城邦项目的关注！本指南说明如何为这个项目做出贡献。

## 行为准则

- 尊重每一位贡献者
- 建设性反馈
- 公共讨论保持专业

## 提交流程

1. **Fork 仓库**
2. **创建特性分支**：`git checkout -b feature/my-feature`
3. **提交前**：
   - [ ] 运行 `make lint && make test`
   - [ ] 更新相关文档
   - [ ] 如有重大决策，创建 ADR（`docs/adr/`）
4. **提交**：`git commit -m "feat: ..."`（遵循 Conventional Commits）
5. **推送并开 PR**

## Commit 规范

```
<type>(<scope>): <subject>

type: feat | fix | docs | refactor | test | perf | chore
scope: agent-os | world-engine | api-gateway | ...
```

示例：
- `feat(agent-os): add chat_turns counter`
- `fix(world-engine): correct AABB edge case`
- `docs(api): update saga endpoint spec`

## PR 规范

- 一个 PR 一个变更
- 标题清晰，描述含动机 + 实现 + 测试
- 关联 Issue / ADR 编号
- 等待 CI + 至少 1 名 code owner 审批

## 代码规范

- Python：Ruff + mypy strict
- TypeScript：ESLint + Prettier
- Rust：Clippy + rustfmt
- Go：golangci-lint + gofmt

## ADR 何时创建

当你做出以下决策时，必须开 ADR：
- 引入新的技术栈 / 库
- 改变跨服务接口
- 改变数据 schema
- 改变部署架构
- 撤换核心依赖

模板见 `docs/adr/template.md`。

## 故障与事故

线上故障处理：
1. 立即在 Slack #incident 通报
2. 优先止血（限流 / 降级 / 回滚）
3. 写 post-mortem（`docs/post-mortem/`）
4. 关联 ADR（如有架构改进）

## 文档维护

- 所有新代码必须有 README + 关键注释
- 重大变更同步更新对应主文档（`docs/01-14`）
- 文档不全的 PR 视为不完整

## 联系方式

- Slack: #aicity-dev
- Email: dev@aicity.dev
- Issue: GitHub Issues

---

> **哲学**：**先求"不崩"再求"好玩"**。
> 所有"应该没问题"的判断，必须被测试**证明**没问题。
