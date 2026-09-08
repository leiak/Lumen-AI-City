# Sprint 12 Min-Slice E2E Verification Report

**Date:** 2026-09-08
**Scope:** T01–T05 + wire-contract double-wrap fix + docker-compose pre-work (T06)
**Status:** Implementation DONE — Full E2E requires manual `docker compose up` + browser

## Implementation Summary

| Series | Tasks | Status | Commits |
|---|---|---|---|
| T01 (agent-os) | T01a–T01e | DONE | `2988278`, `3787268`, `d3c0836`, `48b49cf`, `81ce4e8`, `e419367` |
| T02 (ws-gateway) | T02a–T02c | DONE | `dbc095f`, `e7fa566`, `409e285` |
| T03 (web) | T03a–T03e | DONE | `50fc45a`, `1981256`, `20bd3ca`, `9974f90`, `87f8bb9`, `177ef3c` |
| T04 (templates) | T04a–T04b | DONE | `20a9a93`, `d3d8302` |
| T05 (api-gateway) | T05a–T05c | DONE | `074ade0`, `af0e547`, `d1a89ee` |
| Wire-contract fix | — | DONE | `b90e8a0` (agent-os), `53a753d` (api-gateway) |
| Docker pre-work | T06-A | DONE | `890ef4b` (this PR) |

Total: 23 commits, ~1900 lines of new code across 4 services.

### What landed

- **agent-os** (Python 3.12 + FastAPI) — NpcRegistry yaml loader, ActionDispatcher, SayScheduler
  background asyncio.Task, lifespan-managed RedisPub, OCEAN personality schema
- **ws-gateway** — `RunMultiSubscriber` multi-channel support, per-channel `Fn(String) -> Option<T>`
  filter, `npc_dialogue` channel alongside `player_moved`
- **api-gateway** — `npc.Tree` parser, `POST /v1/npc/talk` handler, NPC_001/002 error codes,
  best-effort Redis publish
- **web** — `api.postNpcTalk` client, `npc_dialogue` event bridge → `NPCDialog` component,
  WorldMap NPC marker rendering + click-to-open dialog
- **templates** — `wang_boss.yaml` (OCEAN + 3 greetings + schedule; `talk_tree` deferred to Sprint 13+),
  `lihua.yaml` (second NPC fixture, future use), `OCEAN-schema.json`

### Wire-contract fix

Originally both services wrapped the payload in an outer `{type, payload}` envelope before
publishing. ws-gateway already wraps every published message uniformly with
`{type, trace_id, ts_ms, payload}` (see `internal/hub/protocol`), so the inner envelope
became a `payload.payload.npc_id` double-wrap that web's `ws-events.ts` had to peel back.

Fix: both publishers (`agent-os` `ActionDispatcher` + `api-gateway` `npc_talk.go`) now
publish the **inner payload only** (the same shape world-engine uses for
`aicity:player:moved`). ws-gateway wraps it once. Web reads `frame.payload.npc_id`
directly. Single source of truth for the envelope lives in
`apps/ws-gateway/internal/hub/protocol`.

## Pre-Work Done in This Task (T06-A)

1. **`apps/agent-os/Dockerfile`** — rewritten to T01e spec
   - `python:3.12-slim` base + `uv` system install + `uv pip install --system`
   - `EXPOSE 8084` (not 8000)
   - Default env: `REDIS_URL`, `REDIS_CHANNEL_NPC_DIALOGUE`, `NPC_TEMPLATES_DIR=/etc/aicity/npc-templates`,
     `HTTP_PORT=8084`, `LOG_LEVEL=info`, `SERVICE_NAME=agent-os`, `SAY_TICK_SECONDS=5.0`
   - `PYTHONUNBUFFERED=1` so compose logs stream startup banner immediately
   - `CMD ["python", "-m", "agent_os.main"]` — same entry as local `uv run agent-os`
2. **`docker-compose.yml`** — added `agent-os` service + extended `api-gateway`
   - `agent-os`: port `8084:8084`, env (above), bind mount
     `./packages/npc-templates:/etc/aicity/npc-templates:ro`, `depends_on: redis (healthy)` +
     `api-gateway (service_started)`, `wget /healthz` healthcheck (slim 镜像无 curl)
   - `api-gateway`: added `NPC_CONFIG_DIR=/etc/aicity/npc-templates` (consumed by
     `internal/config/config.go:41`), `REDIS_CHANNEL_NPC_DIALOGUE=aicity:npc_dialogue`
     (槽位留好；当前 main.go:117 写死，留 env 便于后续改 env-driven), 同 bind mount
   - 头部注释从 "7 容器" 改 "8 容器"
3. **YAML validated** — `python -c "yaml.safe_load(...)"` 解析通过；8 services 全部存在，
   env keys / volumes / depends_on 全部按预期

## Manual E2E Checklist (run after this PR)

> 在 host 上执行。`docker compose up -d --build` 启动后，按顺序走完 A→E。

### A. Stack bring-up

```bash
cd /d/work-ai/0401-town/ai-city
docker compose up -d --build
docker compose ps
```

**Expected:** 8 containers — `postgres` / `redis` / `world-engine` / `api-gateway` /
`a2a-gateway` / `ws-gateway` / `web` / **`agent-os`** —— 全部 `(healthy)` 或 `running`
(web 是 `running`，无 healthcheck)。

如果 `agent-os` 一直 `starting`，99% 是 healthcheck 还没轮过 —— 等 10s；
如果 30s 后仍 `starting`：
```bash
docker compose logs agent-os
```
看 startup banner 是否打出来（`agent-os starting` + `npc_templates_dir`）。

### B. Backend curl smoke

```bash
# 1. Health checks
curl -fsS http://localhost:8084/healthz   # agent-os   → {"status":"ok"}
curl -fsS http://localhost:8080/health     # api-gateway → "OK"
curl -fsS http://localhost:8082/readyz     # ws-gateway
curl -fsS http://localhost:50052/readyz    # world-engine

# 2. Login (POST /v1/auth/login)
TOKEN=$(curl -fsS -X POST http://localhost:8080/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"demo","password":"demo123"}' | jq -r .token)

# 3. POST /v1/npc/talk (NPC reply path)
#    取 demo player uuid（每次 down -v 重建会变）
DEMO_ID=$(docker compose exec -T postgres psql -U aicity -d aicity -tAc \
  "select id from player where username='demo';" | tr -d '\r')

curl -fsS -X POST http://localhost:8080/v1/npc/talk \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d "{\"npc_id\":\"npc_wang_boss_001\",\"player_id\":\"$DEMO_ID\",\"choice_id\":\"ask_business\"}"
```

**Expected response** (shape):
```json
{
  "npc_id": "npc_wang_boss_001",
  "player_id": "<demo-uuid>",
  "tile_id": "tile_0_0",
  "say": "...",
  "options": [{"id": "...", "text": "..."}],
  "reply_to_choice_id": "ask_business"
}
```

> **Note:** 当前 `wang_boss.yaml` 只配了 `say.greeting` 三条；`talk_tree` 留给 Sprint 13+。
> 所以 `POST /v1/npc/talk` 在 min-slice 范围会返 `NPC_002 "unknown choice_id: ..."`。
> 这一步的核心是验 200/401/404 的 status code mapping 和 reply envelope 形状，不是验台词。
> 看 `cmd/main.go:117` 把频道 hardcode 为 `aicity:npc_dialogue` —— 即便 NPC_002，
> Redis publish 也应该走通（除非 main.go 改成 fail-fast，留意 logs）。

### C. Redis publish verification (npc_dialogue channel)

```bash
# 一个终端订阅
docker compose exec -T redis redis-cli SUBSCRIBE aicity:npc_dialogue
```

**Expected within 10s** (agent-os `SAY_TICK_SECONDS=5.0` 触发一轮 active say)：
```
1) "aicity:npc_dialogue"
2) {"npc_id":"npc_wang_boss_001","player_id":"","tile_id":"tile_0_0",
    "say":"来了您嘞！今儿喝点什么？","options":[],
    "reply_to_choice_id":null,"ts_ms":1700000000000,"trace_id":"..."}
3) (nil)
```

**Verify the wire contract**:
- 顶层**不**应有 `"type"` 字段 —— ws-gateway 才会包 envelope
- `reply_to_choice_id: null` 表明这是 active say（不是 reply-to-choice）
- `options: []` —— active say 不带选项

### D. Browser flow (manual)

1. 打开 `http://localhost:3000`，login as `demo / demo123`
2. 落到 `/city` —— WorldMap 渲染 3×3 网格，王老板 NPC marker 在 `tile_0_0`
   （参 `web/src/lib/npc-positions.ts`，目前 2 个 NPCs 配置，但只渲染 王老板 marker）
3. 点 NPC marker → NPCDialog 打开，显示 greeting（"来了您嘞！..."）
4. （min-slice 不带 talk_tree 选项，所以 4 仅作 visual 验证）
5. 等 5s → agent-os tick fires → 当前 NPCDialog 的 active-say 自动出现新台词
6. DevTools → Network → WS → 看 frame：
   - `msg.type === "npc_dialogue"` ✓
   - `msg.payload.npc_id` 是 string（**不**是 `msg.payload.payload.npc_id`）—— 验 wire-contract fix
   - `msg.payload.options` 是 `[]` 或 `{id, text}[]`

### E. Wire-contract sanity check (per WS frame)

对收到的每条 `npc_dialogue` frame：

| 断言 | 期望 |
|---|---|
| `msg.type` | `"npc_dialogue"` (string) |
| `msg.payload.npc_id` | string（非 undefined） |
| `msg.payload.say` | string（非空） |
| `msg.payload.tile_id` | string（`"tile_0_0"`） |
| `msg.payload.options` | array（active say 是 `[]`） |
| `msg.payload.reply_to_choice_id` | `null`（active）或 string（reply） |
| `msg.payload.ts_ms` | number |
| `msg.payload.trace_id` | string |
| `msg.payload.payload` | **undefined** —— 没双层 wrap |

如果 `msg.payload.payload.npc_id` 出现 → wire-contract 回归，立刻看
`apps/agent-os/src/agent_os/action_dispatcher.py` 是否又加了外层 envelope。

## Known Limitations (Sprint 13+ scope)

- **OCEAN personality fields not used yet** —— agent-os 解析 `personality.ocean`，但不参与台词生成
- **talk_tree only on api-gateway side** —— agent-os 只用 `say.greeting` 随机；
  api-gateway 的 `/v1/npc/talk` 也只能返 `NPC_002`（YAML 还没补 talk_tree 节点）
- **No player position tracking** —— `npc_dialogue` 事件里 `player_id=""`（active say 没人触发）
- **Hardcoded NPC positions** —— `web/src/lib/npc-positions.ts` 目前 2 个 NPCs 配置，但只渲染 王老板 marker；后续从 world-engine fetch
- **5s tick interval** —— 无事件驱动；玩家进 tile 不会主动触发 greeting
- **No multi-NPC UI** —— 仅有 `npc_wang_boss_001` 一个 marker；`lihua.yaml` 留作 fixture
- **Wang_boss tile id mismatch 风险** —— `home_tile_id: tile_0_0`（与 web 端硬编码一致），
  Sprint 13 接 world-engine fetch 后必须改用真坐标

## Test Coverage

| Service | Unit | Integration | E2E | Status |
|---|---|---|---|---|
| agent-os | 22 pass, 2 skip (Redis gated) | (env-gated) | — | done |
| ws-gateway | all pass | (Redis gated) | — | done |
| api-gateway | all pass (含 npc_talk_test.go) | — | — | done |
| world-engine | (unchanged) | — | — | — |
| web | 23 pass | — | manual | done |

## Risk / Follow-up

1. **`docker compose build agent-os` 首次可能慢或失败** —— uv workspace 解析对 build context 敏感；
   Dockerfile 已固定走 root context + 显式 COPY `apps/agent-os/{pyproject.toml,src}`。
   如果 `uv pip install --system` 报版本冲突，回退方案：把 pyproject.toml 拷到根目录
   或者生成 `requirements.txt` 后改 `pip install -r`。
2. **NPC YAML schema validation 是 best-effort** —— `npc.LoadAll` 解析失败只 warn 不 fail。
   生产环境建议加 fail-fast mode（`NPC_CONFIG_STRICT=1`）或者 schema 校验中间件。
3. **`aicity:npc_dialogue` 在 api-gateway hardcode** —— `cmd/main.go:117` 写死。
   与 agent-os 的 env 名 `REDIS_CHANNEL_NPC_DIALOGUE` 不一致。
   Trivial follow-up：从 env 读、改 main.go 一行即可。
4. **`depends_on: api-gateway (service_started)`** —— 当前 agent-os 不真依赖 api-gateway
   （无 HTTP 调用），只是直观表达启动顺序；如果觉得多余可删。

## Files Changed in T06-A

- `apps/agent-os/Dockerfile` — full rewrite (T01e spec)
- `docker-compose.yml` — `agent-os` service + `api-gateway` 增 env/volumes + 头部注释
- `docs/superpowers/reports/2026-09-08-sprint12-min-slice-e2e.md` — this report

Commit: `890ef4b` (Dockerfile + compose)