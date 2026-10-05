# 2.0 阶段 2 — 流式 emotion 演示录屏

> **状态**: 部分录制（Partially Recorded）
> **录制日期**: 2026-10-05
> **录制者**: T30 implementer (claude-code, headless Windows server)
> **录制环境**: docker compose 全栈 + Playwright headless Chromium
> **目标**: 按 [2.0-stage2-recording-script.md](../2.0-stage2-recording-script.md) 1-min 3 镜头

## 录制结论

由于 headless Windows server 无 GUI + 无 ANTHROPIC_API_KEY + a2a-gateway 容器冲突，**完整的流式 + emotion overlay 录屏无法在自动化中复现**。本次实际捕获内容：

| Shot | 计划 | 实际 | 状态 |
|---|---|---|---|
| Shot 0 | landing → /city | landing → /login → 填表 → 提交 | 完成（铺垫）|
| Shot 1 | 王老板流式对话（句子级 + 8 类 emotion） | 王老板 NPCDialog 弹窗（含问候语 + 3 选项 + 输入框）| 部分（对话流式未触发，因无 LLM）|
| Shot 2 | 5 NPC emotion overlay 并列 | 截图显示 3 NPC dots（王老板 / 李华 / 张奶奶），seed 只有 3 NPC 而非 5 | 缺失（seed 不全）|
| Shot 3 | acceptance_2_1 5/5 PASS | acceptance_2_1.exe 二进制存在但容器无法启动 | 缺失（容器冲突）|

**录制结果文件**：
- [`streaming-demo.webm`](./streaming-demo.webm) — 853 KB，约 30 秒 headless 浏览器录屏（含登录 + WorldMap + NPC 弹窗）
- [`landing-page.png`](./landing-page.png) — 首页
- [`login-filled.png`](./login-filled.png) — 登录表单填写完毕
- [`worldmap-with-dialog.png`](./worldmap-with-dialog.png) — WorldMap 含 3 NPC dots + 王老板 NPCDialog 弹窗

## 录制方法

```bash
# 自定义 Playwright recorder（web/recorder.mjs）：
# 1. 打开 / 等 3s
# 2. 打开 /login 填 demo/demo123 截图
# 3. 跳 /city 等 5s 截图（NPC 元素 data-npc-id）
# 4. 强制点击 NPC dots（部分失败，因 SVG 坐标 cx=-50 在视口外）
# 5. 截图最终帧
node recorder.mjs docs/2.0-demo/capture/
```

浏览器自动化使用 `chromium.launch({ headless: true })` + `recordVideo: { size: { width: 1920, height: 1080 } }`。视频输出 VP8/VP9 webm，30s 内容压到 853KB。

## 限制与重做建议

### 阻塞点 1: 5 NPC seed 不全

```bash
$ docker exec aitown-postgres-1 psql -U aicity -d aicity -tAc \
    "SELECT count(*) FROM npc;"
3   # 期望 5
```

stage 2 脚本的 5 NPC（王老板 / grace_healer / snack_owner / book_keeper / dance_leader）只有 3 个在 seed（王老板 / 李华 / 张奶奶）。`scripts/seed-npcs.sql` 未补齐 grace_healer / snack_owner / book_keeper / dance_leader 4 行，需要在重新录制前执行：

```bash
docker exec -i aitown-postgres-1 psql -U aicity -d aicity < scripts/seed-npcs.sql
```

或重做 `docker compose down -v && docker compose up -d --build` 让 initdb 重灌 seed。

### 阻塞点 2: 无 ANTHROPIC_API_KEY

stage 2 流式 + emotion overlay 依赖 LiteLLM → Claude Sonnet 4.6 真 LLM 响应。本地 `.env` 未注入 `ANTHROPIC_API_KEY`，导致：

- Shot 1 NPCDialog 流式响应（句子级 + emotion 标签）→ **不会触发**
- Shot 2 5 NPC 并列流式 → **不会触发**

无 key 时的应急路径：使用 `scripts/trigger-npc-stream-loop.py` 走 mock LLM 路径（per T29 计划），该脚本向 Redis 频道 `aicity:npc:say_stream` 直接 publish 预制 beats，跳过 LiteLLM：

```bash
python scripts/trigger-npc-stream-loop.py
# 5 NPC beats 循环，每 3s 一轮：<emotion=happy>来了您嘞！</emotion> 等
```

但 web UI 端的 NPCDialog `sentences` 数组需要 chat-input 提交触发，而 mock loop 走主动 say 路径（welcome 流），两者表现不同。要完整重做需要：

1. 注入真实 `ANTHROPIC_API_KEY` 到 `.env` 然后 `docker compose up -d --build`
2. 或写一个 mock 流式路径（点 NPC → chat-input → 通过 `/v1/npc/:id/talk` 走 mock dispatcher 返回预制 beats）

### 阻塞点 3: a2a-gateway 容器冲突

```bash
$ docker compose up -d a2a-gateway
Error response from daemon: Conflict. The container name "/aitown-postgres-1" is already in use by container "273888d50ae7f8ec9aaf41c60ef4ba5cd39b53740fc8690de804190fe53e2a27"
```

postgres 容器已经在跑但没纳入 compose 网络。重启 a2a-gateway 需要：

```bash
docker compose down a2a-gateway  # 只 down 这个容器
docker compose up -d a2a-gateway  # 再起
```

或 hard reset：

```bash
docker compose down -v  # 清卷，会重灌 seed
docker compose up -d --build
```

### 阻塞点 4: headless Windows server 无显示

不能直接用 `npx playwright test --headed`。已用 headless 模式 + `recordVideo` API 解决，但代价是缺乏真鼠标移动 / 摄像头旁白。完整 demo 录制需要：

- macOS: QuickTime Player 屏幕录制 + 摄像头
- Windows: OBS Studio 屏幕 + 摄像头
- Linux: ffmpeg `-f x11grab`

## 后续 Action

T31 收尾时，此 placeholder 文件与 853KB webm 一起保留。重做建议：

1. 注入 `ANTHROPIC_API_KEY` 到 `.env`
2. `docker compose down -v && docker compose up -d --build`
3. 确认 5 NPC seed（执行 `scripts/seed-npcs.sql` 补齐）
4. 在 macOS / Windows GUI 上跑 `docs/2.0-stage2-recording-script.md` 脚本
5. 压缩到 ≤ 3MB 上传 YouTube unlisted + B 站
6. README banner + `docs/2.0-ROADMAP.md` 演示视频章节替换占位符

## 参考

- [2.0-stage2-recording-script.md](../2.0-stage2-recording-script.md) — 1-min 镜头脚本（计划 3 镜头）
- [2.0-recording-script.md](../2.0-recording-script.md) — 阶段 1 5-min 脚本（应急参考）
- [1.0-demo-package/recording-guide.md](../1.0-demo-package/recording-guide.md) — 录制工具详解
- plan: `docs/superpowers/plans/2026-10-05-2.0-stage2-stream-emotion.md` §T30