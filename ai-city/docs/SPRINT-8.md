# Sprint 8 复盘

> 范围：**web 玩家端 — MapLibre → SVG，接到 world-engine；浏览器真客户端链路打通**
>
> 完成时间：2026-09-07
>
> 提交：
> - `8f0547a` fix(api-gateway): 补 CORS 中间件 —— 浏览器登录一直是失败的
> - `e38a860` feat(web): MapLibre → SVG 地图，接到 world-engine（/v1/tiles + /v1/world/move）
> - `cb92ac3` test(web): tileIdAt 单测 + 修 WorldMap 两处小问题
> - `8d471b3` fix(web): 补 postcss 配置 + tile label 挪出 y-flip 组
> - `335fa35` test(web): Playwright E2E — login + 9 tile + click move

## 一、本次交付

Sprint 7+ 把 a2a 协议层做完了；Sprint 8 的目标是**让 web 玩家端真把世界画出来**。
结果发现这条路径上 5 个"curl 看着全绿、浏览器全黑"的坑，全部一次扫清。

### A. api-gateway CORS 中间件（Sprint 8 前置）

容器化后 web(:3000) 跨源调 api-gateway(:8080)，而 api-gateway 从来没有任何 CORS
处理。预检 OPTIONS → 404，真实 POST 返 200 但不带 `Access-Control-Allow-Origin`，
浏览器直接丢弃响应 → 登录失败。

跟之前几 sprint 是**同一类洞**：从来没被真实客户端走过的路径，curl 不受 CORS 约束，
命令行验收全是绿的，完全看不出来。

| 项 | 说明 |
|---|---|
| 文件 | `apps/api-gateway/internal/middleware/cors.go` + `cors_test.go`（7 case）|
| allowlist | env `CORS_ALLOWED_ORIGINS`，默认 `http://localhost:3000`（compose 显式声明）|
| 中间件顺序 | CORS 在 RateLimit/AntiScrap **之前**：预检不带 Authorization，会被反爬/限流拦掉 |
| 实现选择 | 手写而**不**引 `gin-contrib/cors` —— 与本仓库手写 redis/pg client 风格一致 |
| 回显 Origin | 按 allowlist 回显，不用 `"*"`（`"*"` 与 credentials 互斥，将来上 cookie 会失效）|
| 预检头 | `Allow-Headers` 必须含 `Authorization`，否则鉴权路由在浏览器里全调不通 |
| Vary | 恒发 `Vary: Origin`，避免共享缓存把一个源的响应喂给另一个源 |
| 失败模式 | 未授权 origin 预检 403、不发 `Allow-Origin`；`cors_test.go` 守住 |

### B. web：MapLibre → SVG 地图

之前 MapLibre 配 demotiles 的北京底图与 AI 城邦无关；这次改成自带的 SVG（无新依赖），
轮询 `/v1/tiles` 拉 9 个 tile，点空地发 move，并乐观更新 + 失败回滚。

| 项 | 说明 |
|---|---|
| 删 | `maplibre-gl` + `@aicity/client-reconciler`（transpilePackages + package.json + pnpm-lock + Dockerfile COPY 4 处同步清掉）|
| 新核心 | `web/src/components/Map/WorldMap.tsx`：viewBox 用世界坐标 `[-100,-100 300 300]`，y 翻转 `scale(1,-1)` 一处搞定 |
| 轮询 | 3s `setInterval`（ws-gateway 还是 stub，轮询是当下唯一办法）|
| 点击 → move | `<svg onClick>` 判 `e.target.tagName` 避开 polygon/circle；`getScreenCTM().inverse()` 反推坐标 |
| 失败回滚 | 乐观更新 `setMyPos(target)`；move 失败回滚到 prev；服务端校正 `resp.x/y` 落 store |
| api.ts | 加 `getTiles` / `move`；module load 时从 `localStorage.aicity_token` 同步 token（修刷页面就丢 token 的隐性 bug）|
| store/game.ts | `playerId` 从 localStorage 读（不再硬编码 `player_001`）|
| PlayerHUD | 改读 `useGameStore`（不再 useState 硬编码）|
| city/page.tsx | `MapView` → `WorldMap`；去掉 `dynamic({ ssr: false })` —— 不引 maplibre-gl、不访问 window，可以 SSR |
| tile id 公式 | 保持 `Math.floor(x/100)`（与 world-engine `Tile::from_xy` 一致）；plan 提议的 `(x+50)/100` 会让 `x∈[-50,0)` 误判为 tile_0_0，未采用 |

### C. tileIdAt 抽模块 + 7 个单测（守护不变量）

| 项 | 说明 |
|---|---|
| 新文件 | `web/src/lib/world-coords.ts`：`tileIdAt(x, y)` + `TILE_SIZE = 100` + `worldToSvgY` |
| 测试 | `web/src/lib/world-coords.test.ts` 7 用例：tile centers / world-engine 测试点复用 / `x=99/100` / `x=-1/-50/-100/-100.001` / `worldToSvgY` 翻号 |
| 价值 | 改 web 这边公式忘了同步 world-engine（或反过来）会立刻 fail |
| WorldMap 同步 | 改用同一份 `tileIdAt`，消除两份实现的隐患 |

### D. PostCSS 配置 + tile label 挪出 y-flip 组

| 项 | 说明 |
|---|---|
| bug | 项目只装 `tailwindcss` 没装 `postcss`/`autoprefixer`，也没 `postcss.config`；`@tailwind utilities` 不展开，**所有 utility class 实际不生效**（h-screen、relative、w-full 全废）|
| 表现 | city 页外层 div 只占内容自然高度 149px（不是 100vh）；SVG click bbox 漂移；e2e 点击命中点 ≠ viewBox 中心 |
| 诊断 | 浏览器 `document.styleSheets` 只有 `:root` + `body` + `.map-container` 3 条规则 —— **毫无 Tailwind 输出** |
| 修复 | `pnpm add -D postcss autoprefixer` + `web/postcss.config.mjs`（tailwindcss + autoprefixer 两插件）|
| tile label | 文字标签从 y-flip `<g>` 内挪出（`scale(1,-1)` 会把文字翻倒），用 `-(center_y - HALF + 4)` 单独算 SVG 正常坐标 y，世界 tile 顶部对应 SVG 顶部 |

### E. Playwright E2E（首支）

| 项 | 说明 |
|---|---|
| config | `web/playwright.config.ts`：baseURL=`http://localhost:3000`（不是 127.0.0.1：CORS allowlist 只放 localhost）|
| 测试 1 | login → /city → 9 tile 渲染 → 静态建筑 / NPC / 自己标记 spot-check → API 预置 demo 到 tile_0_0 → 点 tile_-1_0（永远空）→ assert HUD 坐标到 `(-50, 50)` |
| 测试 2 | login 失败：错误条 `.bg-red-900/30` 出现，含 `/API\|Failed/i` |
| 关键技巧 | 点空地用 `dispatchEvent(new MouseEvent('click', {bubbles: true, clientX/Y}))` 在 tileG 上 dispatch，绕开 Playwright hit-test（tile 中心 NPC / 其它 player 圆点会拦截 `.click()` 30s 超时）|
| 结果 | 2/2 通过；前置 vitest 7/7 也通过 |

## 二、E2E 输出

```bash
$ docker compose ps
NAME                    STATUS              PORTS
aitown-a2a-gateway-1    Up (healthy)        0.0.0.0:8083->8083, 0.0.0.0:50061->50061
aitown-api-gateway-1    Up (healthy)        0.0.0.0:8080->8080
aitown-postgres-1       Up (healthy)        127.0.0.1:5432->5432
aitown-redis-1          Up (healthy)        127.0.0.1:6379->6379
aitown-web-1            Up                  0.0.0.0:3000->3000
aitown-world-engine-1   Up (healthy)        0.0.0.0:50051-50052

$ cd ai-city/web && pnpm exec vitest run
Test Files  1 passed (1)
     Tests  7 passed (7)
  Duration  1.55s

$ pnpm exec playwright test
Running 2 tests using 1 worker
initial HUD: x=50, y=50
will click tile_-1_0 center (-50, 50)
  ✓  1 [chromium] › e2e/map-flow.spec.ts:11:5 › login → /city → 9 tiles visible → click move → position changed (4.3s)
  ✓  2 [chromium] › e2e/map-flow.spec.ts:140:5 › login failure shows error message (527ms)
2 passed (5.8s)

$ TOKEN=$(curl -s -X POST http://localhost:8080/v1/auth/login -H 'Content-Type: application/json' -d '{"username":"demo","password":"demo123"}' | jq -r .token)
$ curl -s http://localhost:8080/v1/tiles -H "Authorization: Bearer $TOKEN" | jq '.[] | select(.player_ids or .npc_ids) | {id, players: .player_ids, npcs: .npc_ids}'
{
  "id": "tile_0_-1",  "players": ["1fce8ddd-…"], "npcs": []        # demo 移动后
}
{
  "id": "tile_0_0",   "players": [],            "npcs": ["npc_wang_boss_001"]
}
{
  "id": "tile_1_0",   "players": ["grpc_smoke_player_001"], "npcs": ["npc_lihua_001"]
}
{
  "id": "tile_-1_1",  "players": [],            "npcs": ["npc_zhang_granny_001"]
}
```

## 三、关键决策

| # | 决策 | 理由 |
|---|---|---|
| 1 | CORS 手写而不引 `gin-contrib/cors` | 与本仓库手写 redis/pg client 风格一致；也避免动 go.work.sum |
| 2 | CORS 顺序在 RateLimit/AntiScrap 之前 | 预检不带 Authorization，会被反爬/限流拦掉 |
| 3 | CORS 回显 Origin 而不用 `"*"` | `"*"` 与 credentials 互斥，将来上 cookie 会失效 |
| 4 | 删 `maplibre-gl`（约 800KB） | Mercator 渲染器与 AI 城邦世界坐标无关；用自带 `<svg>` + `<polygon>` + `<rect>` 即可 |
| 5 | 删 `@aicity/client-reconciler` | `nextMove()` 只是 `++this.sequence` 凑 MoveRequest，无预测/校正/回滚逻辑，对直 HTTP 调用无价值 |
| 6 | 渲染选 SVG 而非 Canvas/WebGL | 世界是 9 个静态 tile + 几个标记，无重绘压力；SVG 的 `<g>` 分组 + `data-*` 属性方便测试 |
| 7 | 坐标系：viewBox 直接用世界坐标 `[-100,-100 300 300]`，**不缩放** | 简化代码：tile rect / 建筑 polygon 都在世界坐标里加常量偏移即可；y 翻转 `scale(1,-1)` 一处搞定 |
| 8 | tile id 公式保持 `Math.floor(x/100)` | 与 world-engine `Tile::from_xy` 完全一致；`(x+50)/100` 会让 `x∈[-50,0)` 误判 tile_0_0 |
| 9 | `tileIdAt` 抽到独立 `world-coords.ts` | WorldMap 和测试共用一份；改公式单测立刻 fail |
| 10 | 轮询 3s 而非 WebSocket | ws-gateway 还是 stub；轮询当下是唯一办法；Sprint 9+ 接入 ws 后再切 |
| 11 | 失败回滚：move 失败回 `prev`，服务端校正 `resp.x/y` 落 store | world-engine + Redis + PG 真在改世界，"假装没动" 不可接受 |
| 12 | 点击判 `e.target.tagName` 避开 polygon/circle | 留出"点建筑 / 点 NPC"给后续 chat 交互 |
| 13 | token module load 时从 localStorage 同步 | 修刷页面就丢 token 的隐性 bug；JWT 不能 inline，只能 localStorage |
| 14 | postcss.config.mjs 而非 .js | Next.js 15 推荐 ESM；与 `package.json` 现有 `"type"` 风格一致 |
| 15 | Playwright 用 `dispatchEvent` + MouseEvent 绕 hit-test | NPC / 其它 player 圆点在 tile 中心，`click()` 会被拦截 30s 超时；用 dispatch 让 e.target 保持 tileG |
| 16 | Playwright baseURL=`localhost:3000` 不 `127.0.0.1:3000` | api-gateway CORS allowlist 只放 localhost；浏览器看到两个不同源 |
| 17 | e2e 测试先调 API 重置 demo 到 tile_0_0 | 上一次测试可能把 demo 移到任意 tile；preset 让断言确定性 |
| 18 | 选 tile_-1_0 做 click 目标 | 永远空（无 NPC / 无 player / 无 building）；其它 tile（tile_1_0、tile_0_0）会被常驻 NPC 或上次残留 player 拦截 |

## 四、变更清单

### 新增（6）

- `web/src/lib/world-coords.ts` —— `tileIdAt` + `TILE_SIZE` + `worldToSvgY` 单一实现
- `web/src/lib/world-coords.test.ts` —— 7 用例护栏
- `web/vitest.config.ts` —— `environment: 'node'` 跑 ts 单测
- `web/postcss.config.mjs` —— tailwindcss + autoprefixer 插件
- `web/e2e/map-flow.spec.ts` —— 2 用例（login+move / login-fail）
- `web/playwright.config.ts` —— baseURL=localhost:3000, headless chromium

### 修改（11）

- `apps/api-gateway/internal/middleware/cors.go` —— 新增（按上面分类也算"修改"）
- `apps/api-gateway/internal/middleware/cors_test.go` —— 7 用例（预检/Origin 拒绝/Allow-Headers/Vary/no-Origin/credentials）
- `apps/api-gateway/cmd/main.go` —— `Use(cors.New(cfg.CORSAllowedOrigins))` 注册
- `apps/api-gateway/internal/config/config.go` —— `CORSAllowedOrigins` 字段
- `docker-compose.yml` —— `CORS_ALLOWED_ORIGINS=http://localhost:3000` 显式声明
- `web/src/components/Map/WorldMap.tsx` —— 全新（替换 MapView.tsx 的 MapLibre）
- `web/src/lib/api.ts` —— `getTiles()` + `move()` + module-load token hydrate
- `web/src/store/game.ts` —— `playerId` 从 localStorage 读
- `web/src/components/PlayerHUD.tsx` —— 改读 store
- `web/src/app/city/page.tsx` —— `MapView` → `WorldMap`；去 `dynamic ssr:false`
- `web/next.config.js` —— 去掉 `client-reconciler` from transpilePackages
- `web/package.json` —— 删 `maplibre-gl`；加 `@playwright/test`；加 `postcss` + `autoprefixer` devDeps
- `web/Dockerfile` —— 去两行 `COPY packages/client-reconciler`（manifest + 源码 layer）
- `web/pnpm-lock.yaml` —— regen（删 client-reconciler / maplibre-gl，加 postcss/autoprefixer/@playwright/test）
- `.gitignore` —— 加 `test-results/` `playwright-report/` `playwright/.cache/`
- `docs/SPRINT-8.md` —— 本文件

### 删除（3）

- `packages/client-reconciler/` 整目录（88 行 stub）
- `web/src/components/Map/MapView.tsx`（41 行 MapLibre 整块）
- `web/src/components/Map/` 旧 MapView 引用链

## 五、关键陷阱

| # | 陷阱 | 缓解 |
|---|---|---|
| 1 | **CORS 是"curl 看不见"的洞**：api-gateway 从来没 CORS 处理；curl 跳过预检，命令行验收全绿 | 这次 web 真浏览器走通才暴露；`cors_test.go` 7 case 守住 |
| 2 | CORS 顺序在 RateLimit/AntiScrap 之前 | 文档 + 注释强调"预检不带 Authorization" |
| 3 | `Access-Control-Allow-Headers` 必须含 `Authorization` | 不含则鉴权路由预检失败；测试覆盖 |
| 4 | `Vary: Origin` 必须恒发 | 否则共享缓存把一个 origin 的响应喂给另一个 origin |
| 5 | **Tailwind/PostCSS 一直是断的**：项目只装 `tailwindcss` 没装 `postcss`/`autoprefixer`，也没 postcss.config；`@tailwind utilities` 不展开，**所有 utility class 实际不生效** | 诊断：浏览器 `document.styleSheets` 只有 `:root` + `body` + `.map-container` 3 条规则；`pnpm add -D postcss autoprefixer` + `web/postcss.config.mjs` |
| 6 | tile id 公式不能用 `(x+50)/100`（plan 原提议） | `x∈[-50,0)` 会误判 tile_0_0；保持 `Math.floor(x/100)`，与 world-engine `Tile::from_xy` 一致 |
| 7 | tile 中心圆点拦截 Playwright `.click()` | `page.evaluate` 内 `dispatchEvent(new MouseEvent('click', {bubbles: true, clientX/Y: bbox.center}))` 在 tileG 上发 |
| 8 | Playwright `viewport: {1280, 800}` 被 `devices['Desktop Chrome']` 覆盖回 `1280, 720` | 用 `devices['Desktop Chrome']` 的话 viewport 是 720；要么单独 devices 不用 Desktop Chrome，要么接受 720 |
| 9 | Playwright baseURL 必须 `localhost:3000` 不能 `127.0.0.1:3000` | CORS allowlist 只放 localhost；浏览器把两者当不同 origin |
| 10 | `tileIdAt(-100, 0)` → `tile_-1_0` 不是 `tile_-2_0`（`Math.floor(-1) = -1`） | 测试用例显式覆盖；`(-100.001, 0)` 才进 tile_-2_0 |
| 11 | `world-engine` 的 `corrected_position` 实际**没做碰撞校正**（grpc.rs:114-117 直接回 `target.x/y`）| 容差 1.0 足够；如果将来加碰撞避让，需放宽或重新算 |
| 12 | `e.target.tagName` 区分 polygon/circle 在 y-flip `<g>` 内仍正确 | y-flip 不影响 tagName；只在视觉上翻转 |
| 13 | `npm install --no-frozen-lockfile` 才能加新 deps | frozen-lockfile 在 lock 与 disk 不一致时硬失败；新加 postcss/autoprefixer 后 regen lockfile |
| 14 | **token hydrate 时机**：必须在 `api.ts` module top-level，不能在 React effect 里 | React effect 在 login 后才跑，但 `api.move()` 可能更早（登录成功后立即 fetchTiles）|
| 15 | `client-reconciler` 删后 `next.config.js` 的 transpilePackages 还会引它 | 改 next.config.js → 删包 → 删 Dockerfile COPY 行 → regen lockfile；缺一步 build fail |

## 六、回滚

- **CORS**：单文件 revert 即可（无 schema 变更）
- **SVG 替换 MapLibre**：`git revert e38a860 cb92ac3 8d471b3 335fa35` —— 四提交一齐回
- **删除 client-reconciler**：物理目录已删，`git revert e38a860` 恢复 88 行 stub
- **PostCSS 配置**：单文件 revert；package.json 的 postcss/autoprefixer devDeps 可留（无副作用）
- **Playwright**：单 revert；不动其它

## 七、后续 Sprint 建议

按依赖顺序：

1. **Sprint 8+**：ws-gateway 实装，把 WorldMap 的 3s 轮询切到 ws 推送（position event 流式更新）
2. **Sprint 8+**：玩家"我"的细化渲染 —— `/v1/tiles` 现在只给 `player_ids[]` 没有 `(x, y)`；要么补字段，要么前端再调 `/v1/players/:id/position`
3. **Sprint 8+**：点 NPC / 点建筑 → chat 触发；WorldMap onSvgClick 已经留好 `tag === 'polygon' || tag === 'circle'` 早返回分支
4. **Sprint 8+**：tile 边界平滑 —— 视口只显示 9 个 tile，等地图扩大（10×10+）需要 LOD + viewport 裁剪
5. **远期**：WorldMap 测试覆盖（vitest + jsdom）—— 现在只有 e2e 端到端验，没有组件级单测
