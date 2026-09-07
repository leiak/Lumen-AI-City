# 5min 上手 — Quickstart

> **目标**：5 分钟内启动 8 容器，看到 1.0 闭环。

## 0. 前置

```bash
# 确认 Docker Desktop 跑着
docker --version
docker compose version

# 确认端口未占
python -c "
import socket
for p in [3000, 5432, 6379, 50051, 50052, 8082, 8088, 8083, 50061, 9000]:
    s = socket.socket()
    s.settimeout(0.5)
    r = s.connect_ex(('127.0.0.1', p))
    print(f'{p}: {\"占用\" if r == 0 else \"空闲\"}')"
# 预期：除了 3000 可能被占用（其他 web 项目），其他都应该空闲
```

如果端口被占：
- `3000`：停其他 web 项目，或改 `web/.env` 的 port
- `5432 / 6379`：停本地 postgres / redis
- `50051-8083`：停本地对应服务

## 1. 起容器（30s ~ 1min）

```bash
cd /d/work-ai/0401-town/ai-city
docker compose up -d --build
# 第一次 build 慢（5-10min）；后续只 rebuild 改动的 service

# 看启动进度
docker compose ps
# 预期：8 个 service 都显示 "Up" 或 "Up (healthy)"
# 注：postgres / redis 要 healthy，world-engine 要 healthy，其他 Up 即可

# 等 30s 让所有依赖串起来
sleep 30
docker compose ps
# 再检查一次
```

**预期 healthy 检查**：
- postgres：`pg_isready` 返 0
- redis：`PING` 返 PONG
- world-engine：`/healthz` 返 200
- 其他：进程跑着即可（无 healthcheck 也行）

## 2. 验 demo 玩家（10s）

```bash
# 确认 demo 玩家存在（pgcrypto bcrypt 种子）
docker compose exec -T postgres psql -U aicity -d aicity -tAc \
  "select id, username from player where username='demo';"
# 预期：UUID + demo 一行

# 确认 admin 玩家存在
docker compose exec -T postgres psql -U aicity -d aicity -tAc \
  "select id, username from player where username='admin';"
# 预期：UUID + admin 一行
```

如果 demo 玩家不存在：
```bash
# 检查 initdb 是否跑了（看 init.sql 是否执行）
docker compose logs postgres | grep "initdb"
# 如果 initdb 没跑 → docker compose down -v && docker compose up -d --build
```

## 3. 重启 agent-os（清 welcome set + 加载最新 yaml）

```bash
docker compose restart agent-os
sleep 3

# 看 agent-os 启动日志
docker compose logs --tail=30 agent-os
# 预期：loaded npc templates count=1, listening on :9000
# 注意：只 1 个 NPC（王老板）enabled，其他 4 个 enabled: false
```

**关键检查**：
```bash
# 王老板 enabled？
docker compose exec -T agent-os cat /app/packages/npc-templates/wang_boss.yaml | grep "^enabled"
# 预期：enabled: true

# 其他 NPC 应该 disabled
docker compose exec -T agent-os ls /app/packages/npc-templates/
# 预期：wang_boss.yaml + lihua.yaml + 2-4 个其他 yaml
# 1.0 演示只王老板
```

## 4. 开浏览器（30s）

```
浏览器 → http://localhost:3000/login
```

输入：
- 用户名：`demo`
- 密码：`demo123`

点登录。

**预期**：跳转到 `/city`，看到 9 个 tile 渲染，王老板圆点在 tile_0_0 中心。

## 5. 走 demo 路径（30s）

```
1. 点 tile_1_0（玩家走开）
2. 等 1s
3. 点 tile_0_0（玩家走回王老板附近）
```

**预期 1-3s 内**：NPCDialog 弹气泡，王老板 say "欢迎来到城邦！有什么可以帮你的吗？"

## 6. 跑 acceptance_1_0（30s）

```bash
docker compose exec -T a2a-gateway /app/acceptance_1_0
```

**预期**：
```
Step 1 (login): PASS
Step 2 (walk): PASS
Step 3 (approach NPC): PASS
Step 4 (NPC主动say): PASS
Step 5 (玩家回复): PASS

5/5 PASS, exit 0
```

如果失败 → [acceptance-cheatsheet.md §故障诊断](acceptance-cheatsheet.md#故障诊断)

---

## 故障诊断

### Q：容器起不来 / 卡 starting

**A**：
```bash
# 1. 看具体哪个 service 卡
docker compose ps
# 2. 看卡住的 service 日志
docker compose logs --tail=50 <service-name>
# 3. 常见原因：
#    - postgres / redis 没 healthy → 其他都卡（依赖）
#    - world-engine 缺 dlltool → 检查 PATH（参 README §Windows 工具链）
#    - 端口被占 → 见 §0 前置检查
```

### Q：登录失败 / 401

**A**：
```bash
# 1. 确认 demo 玩家存在（§2）
# 2. 确认 JWT_SECRET 一致（api-gateway + ws-gateway）
docker compose exec -T api-gateway env | grep JWT_SECRET
docker compose exec -T ws-gateway env | grep JWT_SECRET
# 应该都是 dev-secret-change-me（dev 默认）

# 3. 看 api-gateway 日志
docker compose logs --tail=30 api-gateway | grep -E "login|auth|401"
```

### Q：玩家移动后地图不更新

**A**：
```bash
# 1. 看 api-gateway 是不是收了 move 请求
docker compose logs --tail=30 api-gateway | grep -E "world/move"

# 2. 看 world-engine 是不是处理了
docker compose logs --tail=30 world-engine | grep -E "Move|correction"

# 3. 看 ws-gateway 是不是推送了
docker compose logs --tail=30 ws-gateway | grep -E "player_moved|fanned"
```

### Q：NPC 不主动 say（气泡不弹）

**A**：见 [acceptance-cheatsheet.md §故障诊断 — Q2 王老板不主动打招呼](acceptance-cheatsheet.md#q2-王老板不主动打招呼)

### Q：第二 tab 不同步

**A**：
```bash
# 1. 看 ws-gateway 是不是 fanout 了
docker compose logs --tail=30 ws-gateway | grep -E "broadcast|fanout|delivered"
# 预期：delivered=N (N = 客户端数)

# 2. 浏览器 devtools → Network → WS 帧
#    应该看到 player_moved envelope

# 3. 硬刷新第二 tab（Ctrl+Shift+R）
# 4. 还不行 → docker compose restart ws-gateway
```

---

## 完整命令清单（复制粘贴）

```bash
# === 起容器 ===
cd /d/work-ai/0401-town/ai-city
docker compose up -d --build
sleep 30
docker compose ps

# === 验 demo 玩家 ===
docker compose exec -T postgres psql -U aicity -d aicity -tAc \
  "select id, username from player where username='demo';"

# === 重启 agent-os ===
docker compose restart agent-os
sleep 3
docker compose logs --tail=20 agent-os | grep -E "loaded|listening"

# === 跑 acceptance_1_0 ===
docker compose exec -T a2a-gateway /app/acceptance_1_0

# === 停所有 ===
docker compose down

# === 停 + 清数据（重建 initdb）===
docker compose down -v
docker compose up -d --build
```

---

## 下一步

- 跑通 5 步 → 看 [faq.md](faq.md) 准备 stakeholder 提问
- 录屏给 stakeholder 看 → [recording-guide.md](recording-guide.md)
- 深入了解 → [1.0-ROADMAP.md](../1.0-ROADMAP.md)
