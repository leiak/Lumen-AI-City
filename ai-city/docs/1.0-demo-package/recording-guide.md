# 录屏指南

> **目标**：录 3 分钟 demo 给 stakeholder 看，含完整闭环 + intro + outro。

## 录制规格

| 项 | 规格 |
|---|---|
| 时长 | 3:30（intro 30s + 主体 3min + outro 30s）|
| 分辨率 | 1920×1080 (1080p) |
| 帧率 | 30 fps |
| 格式 | WebM（GitHub README 兼容）/ MP4（通用）|
| 文件大小 | ≤ 5MB（README 可嵌）|
| 音频 | 可选（讲解词录制）|

## 录制工具

### Windows（推荐）

```bash
# 1. Xbox Game Bar（系统自带）
# Win + G → 录制按钮
# 录完保存到：~/Videos/Captures/

# 2. OBS Studio（更专业，免费）
# https://obsproject.com/
# 设置：来源 = 显示器捕获 / 窗口捕获
# 输出：格式 = mkv（录制时）→ 重 mux 为 webm
# 控制：开始录制 / 停止录制（快捷键）
```

### macOS（推荐）

```bash
# 1. QuickTime Player（系统自带）
# File → New Screen Recording → 选整屏 or 选窗口
# 录完 File → Export As → 1080p / 格式选 webm 或 mp4

# 2. OBS Studio（同 Windows）
```

### Linux

```bash
# ffmpeg 命令行（无 GUI）
ffmpeg -f x11grab -video_size 1920x1080 -framerate 30 \
       -i :0 -c:v libvpx-vp9 -b:v 2M \
       demo-1.0.webm
# 录完 Ctrl+C 停止

# 或 OBS Studio（GUI）
```

## 录制前准备

```bash
# 1. 关闭无关窗口（防录到敏感信息）
# 2. 关闭通知（Win + N 静音模式 / macOS 勿扰）
# 3. 调暗不必要 tab
# 4. 浏览器全屏（F11）→ 演示更聚焦

# 5. 准备好脚本（[docs/1.0-demo-script.md](../1.0-demo-script.md) §一 时间轴）
# 6. 准备终端 + 浏览器 + 录屏工具 三件套
```

## 录制流程

### Intro（30s）

```
[0:00 - 0:15] 终端：docker compose ps 8 healthy
                念："AI 城邦 1.0 demo — 8 服务健康。"

[0:15 - 0:30] 浏览器：localhost:3000
                念："进入 AI 城邦 Web 端。"
```

### 主体（3min，与 [1.0-demo-script.md §一 时间轴](../1.0-demo-script.md) 完全一致）

15 个镜头，每个 ≤ 30s：

```
0:00  服务全健康 → 0:10 登录页 → 0:20 登录 → 0:30 城市渲染 →
0:40 玩家移动 → 0:50 玩家走回 → 1:00 NPC 主动 say ★ →
1:20 5 选项 → 1:30 玩家回复 → 1:40 reply → 2:00 二级话题 →
2:10 buy reply → 2:20 对话结束 → 2:30 第二 tab 同步 →
2:40 acceptance_1_0 → 2:55 5/5 PASS
```

### Outro（30s）

```
[3:00 - 3:15] 终端：git log --oneline -10
                念："最近 5 个 commit 是 1.0 文档。"

[3:15 - 3:30] 终端：tree ai-city/docs/1.0*
                念："1.0 全套文档：范围 / 验收 / Sprint 拆解 / 决策。"
```

## 后处理

### 裁剪（如有出错片段）

```bash
# ffmpeg 裁剪（保留 1:00-3:00，跳过错误片段）
ffmpeg -i raw-demo.webm -ss 00:01:00 -to 00:03:00 -c copy demo-1.0.webm

# 或用视频编辑软件：DaVinci Resolve（免费）/ iMovie / 剪映
```

### 添加水印（可选）

```bash
# ffmpeg 加文字水印
ffmpeg -i demo-1.0.webm \
       -vf "drawtext=text='AI 城邦 1.0':x=20:y=20:fontsize=24:fontcolor=white:box=1:boxcolor=black@0.5" \
       -c:a copy demo-1.0-watermarked.webm
```

### 压缩（如超过 5MB）

```bash
# ffmpeg 压缩（CRF 28-32 适合 web）
ffmpeg -i demo-1.0.webm -c:v libvpx-vp9 -b:v 1M -crf 32 demo-1.0-compressed.webm

# 或降分辨率到 1280×720
ffmpeg -i demo-1.0.webm -vf scale=1280:720 -c:v libvpx-vp9 -b:v 1M demo-1.0-720p.webm
```

### 导出 2 份

```bash
# 完整版 3:30（带 intro/outro）：demo-1.0-full.webm
# 精简版 3:00（仅主体）：demo-1.0.webm
```

## 文件放置

```bash
# 录屏放到 docs/assets/
mkdir -p ai-city/docs/assets
mv demo-1.0-full.webm ai-city/docs/assets/demo-1.0-full.webm
mv demo-1.0.webm ai-city/docs/assets/demo-1.0.webm

# 加进 README.md（精简版）
echo '<video src="docs/assets/demo-1.0.webm" controls width="640"></video>' >> ai-city/README.md
```

## README 嵌入

```markdown
## 🎬 1.0 演示

<video src="docs/assets/demo-1.0.webm" controls width="640"></video>

完整 3:30 版本：[demo-1.0-full.webm](docs/assets/demo-1.0-full.webm)
```

## 录屏 checklist

- [ ] 录屏工具选定（Xbox / QuickTime / OBS / ffmpeg）
- [ ] 1080p / 30fps / WebM 规格确认
- [ ] 不相关窗口关闭 / 通知静音
- [ ] 录屏工具开录 + 立即开始演示
- [ ] 15 个镜头按时间轴跑完
- [ ] 录完检查：每个镜头清晰 / 文字可读 / 无敏感信息
- [ ] 后处理：裁剪 / 水印 / 压缩
- [ ] 导出 2 份：完整版 + 精简版
- [ ] 放到 `docs/assets/`
- [ ] README.md 嵌入精简版

---

## 应急：录屏中出问题

| 问题 | 处理 |
|---|---|
| NPC 不主动 say | 切到 §应急 1，重启 agent-os，重录最后 1min |
| acceptance_1_0 失败 | 录屏用 -verbose 模式跑一遍作为"修复过程"展示，反而更真实 |
| 浏览器卡死 | 备用浏览器继续 |
| 录屏工具崩了 | 重新录（视频短，3min 重录成本低）|
| 麦克风爆音 | 后期去除音频，只保留屏幕 + 字幕 |

---

## 下一步

- 录完 → 把 `demo-1.0.webm` 发给 stakeholder
- 录屏无法替代现场 demo：约定 stakeholder 现场 demo 时间（30min 演示 + 30min Q&A）
- 录屏后改进意见 → 反馈到 [docs/1.0-demo-script.md](../1.0-demo-script.md)
