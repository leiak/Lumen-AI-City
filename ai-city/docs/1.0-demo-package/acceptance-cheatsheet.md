# acceptance_1_0 命令清单 + 故障诊断

> **位置**：`apps/a2a-gateway/cmd/acceptance_1_0/main.go`
> **详细 spec**：[`docs/1.0-acceptance-design.md`](../1.0-acceptance-design.md)
> **演示脚本**：[`docs/1.0-demo-script.md`](../1.0-demo-script.md)

## 命令

### 基础跑

```bash
# 容器内跑（推荐，依赖都在容器里）
docker compose exec -T a2a-gateway /app/acceptance_1_0
# 预期：5/5 PASS, exit 0
```

### Verbose 模式（看完整 HTTP/WS 帧）

```bash
docker compose exec -T a2a-gateway /app/acceptance_1_0 -verbose
# 输出每步的 HTTP 请求/响应 + WS 帧 + 计时
```

### 单独跑某步（debug 用）

```bash
# 只跑 step 4（NPC 主动 say）
docker compose exec -T a2a-gateway /app/acceptance_1_0 -step=4

# 跳过 step 1（用已有 token）
docker compose exec -T a2a-gateway /app/acceptance_1_0 -skip=1
```

### Keep-alive 模式（CI 用）

```bash
# 跑完不退出，等 stakeholder 验收
docker compose exec -T a2a-gateway /app/acceptance_1_0 -keep-alive
# 5/5 PASS 后挂起，按 Ctrl+C 退出
```

### 本地跑（不进容器）

```bash
# 在 ai-city 根目录
go run ./apps/a2a-gateway/cmd/acceptance_1_0 \
  -api http://localhost:8088 \
  -ws ws://localhost:8082 \
  -world-grpc 127.0.0.1:50051 \
  -player demo
```

---

## 退出码

| 退出码 | 含义 | 行动 |
|---|---|---|
| 0 | 5/5 PASS | 演示成功 |
| 1 | 步骤失败（某 step X/5）| 看 hint 表诊断 |
| 2 | setup 失败（容器没起 / demo 玩家不存在）| 看 §0 前置检查 |
| 3 | bad args（CLI 解析失败）| 跑 `-help` 看用法 |
| 4 | 中断（Ctrl+C）| 重跑 |

---

## 5 步详解

### Step 1（login）
- **做什么**：POST `/v1/auth/login` `{"username":"demo","password":"demo123"}` → 拿 JWT
- **耗时**：≤ 1s
- **失败 hint**：
  - 401 → 玩家不存在 / 密码错
  - 503 → api-gateway 没起 / DB 连不上
  - 5s 超时 → api-gateway 慢（看日志）

### Step 2（walk）
- **做什么**：POST `/v1/world/move` `{"player_id":"...","target_x":150,"target_y":0}` → 玩家走到 tile_1_0
- **耗时**：≤ 2s
- **失败 hint**：
  - 400 → MoveRequest 字段错
  - 503 → world-engine 没起 / gRPC 连不上
  - 5s 超时 → world-engine 慢（看 metrics）

### Step 3（approach NPC）
- **做什么**：POST `/v1/world/move` 玩家走回 tile_0_0（王老板位置）→ 验 tile_id 切换 + agent-os 触发
- **耗时**：≤ 3s（agent-os tick 周期 ≤ 1s + 触发 + Redis publish）
- **失败 hint**：
  - move 失败 → 同 Step 2
  - 5s 后 agent-os 没响应 → 看 agent-os 日志（参 §Q2）

### Step 4（NPC 主动 say）
- **做什么**：WS connect 收 npc_dialogue envelope → 验 envelope 字段（npc_id / say / options）
- **耗时**：≤ 5s
- **失败 hint**：
  - 401 → WS token 错 / WS_GATEWAY 拒
  - 5s 无 envelope → agent-os 没 publish（§Q2）
  - envelope 字段错 → dispatcher.say 实现 bug

### Step 5（玩家回复）
- **做什么**：POST `/v1/npc/:id/talk` `{"choice":"ask_business"}` → 验 reply 字段 + next_options
- **耗时**：≤ 1s
- **失败 hint**：
  - 400 → choice 不在 talk_tree
  - 404 → npc_id 找不到
  - 500 → yaml 加载失败
  - reply 字段空 → talk_tree 配置错

---

## 故障诊断

### Q1：退出码 2（setup 失败）

```bash
# 检查 demo 玩家
docker compose exec -T postgres psql -U aicity -d aicity -tAc \
  "select username from player where username='demo';"
# 空 → initdb 没跑：
docker compose down -v
docker compose up -d --build
sleep 30

# 检查 8 容器
docker compose ps
# 任何 unhealthy → 重启：
docker compose restart <service-name>
```

### Q2：王老板不主动打招呼（Step 4 失败 / 演示中气泡不弹）

```bash
# 1. 看 agent-os 日志
docker compose logs --tail=50 agent-os | grep -E "player_moved|trigger|welcome"
# 预期："player demo entered tile_0_0" + "triggering welcome storyline"
# 实际：无 → 玩家 listener 没工作

# 2. 看 welcome set（重启用）
docker compose exec -T agent-os python -c "
from agent_os.player_listener import PlayerListener
print('welcomed:', PlayerListener.welcomed_players)
"
# 空 → 1.0 接受（重启后正常）

# 3. 王老板 enabled？
docker compose exec -T agent-os cat /app/packages/npc-templates/wang_boss.yaml | grep "^enabled"
# enabled: false → 改成 true + restart

# 4. 重启 agent-os
docker compose restart agent-os
sleep 3
# 让玩家再走开再走回

# 5. 手动触发（应急，最后手段）
docker compose exec -T agent-os python -c "
from agent_os.storyline import trigger_storyline
trigger_storyline('npc_wang_boss_001', 'welcome')
"
# 5s 内玩家端应该看到气泡
```

### Q3：Step 5 reply 字段空

```bash
# 1. 看 wang_boss.yaml talk_tree 配置
docker compose exec -T agent-os cat /app/packages/npc-templates/wang_boss.yaml | grep -A 5 "talk_tree:"
# 验 ask_business 节点有 reply 字段

# 2. 看 yaml schema 校验
docker compose exec -T agent-os python -c "
import yaml, jsonschema
with open('/app/packages/npc-templates/OCEAN-schema.json') as f:
    schema = json.load(f)
# ... validate talk_tree 字段
"
# 校验失败 → 改 yaml

# 3. 看 api-gateway 日志
docker compose logs --tail=30 api-gateway | grep -E "npc.*talk|yaml"
# yaml 加载失败 → 文件权限 / 路径错
```

### Q4：第二 tab 不同步

```bash
# 1. 看 ws-gateway fanout
docker compose logs --tail=30 ws-gateway | grep -E "broadcast|fanout|delivered"
# delivered=N 应 = 客户端数

# 2. 浏览器 devtools → Network → WS 帧
# 应该看到 player_moved envelope

# 3. 硬刷新第二 tab（Ctrl+Shift+R）

# 4. 还不行 → 重启 ws-gateway
docker compose restart ws-gateway
sleep 5
# 重新打开第二 tab
```

### Q5：JWT token 过期（Step 1 偶尔 401）

```bash
# JWT 默认 24h 过期。强制重登：
docker compose exec -T a2a-gateway /app/acceptance_1_0 -force-login
# 或：docker compose restart api-gateway
```

### Q6：容器内存不足

```bash
# 看资源占用
docker stats --no-stream

# 释放：
docker system prune
docker volume prune
# 注：volume prune 会清掉 pg 数据！谨慎
```

---

## 进阶：CI 集成

```bash
# GitHub Actions / GitLab CI 用
docker compose exec -T a2a-gateway /app/acceptance_1_0 -ci-mode
# -ci-mode：JSON 输出 + 退出码语义化（0=PASS / 1=FAIL）
# 输出 example：
# {"step":1,"name":"login","status":"PASS","duration_ms":150}
# {"step":2,"name":"walk","status":"PASS","duration_ms":420}
# ...
```

---

## 下一步

- 跑通 5/5 → 录屏给 stakeholder 看：[recording-guide.md](recording-guide.md)
- 失败诊断 → 看 [`1.0-acceptance-design.md §五`](../1.0-acceptance-design.md)
- 演示流程 → [`1.0-demo-script.md`](../1.0-demo-script.md)
