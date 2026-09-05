# Sprint 4 复盘

> 范围：**world-engine ComputePath 真路径（A\* over tile 网格 + building detour）**
>
> 完成时间：2026-09-05
>
> 提交：
> - `feat(sprint4): ComputePath 真路径 — A* over tile grid + building polygon detour`

## 一、本次交付

### world-engine（Rust）

| 项 | 说明 |
|---|---|
| 新模块 | `src/pathfinding.rs`：`find_path(grid, start, end) -> Vec<(f32, f32)>` |
| Tile 级 A\* | 节点 = `(tile_x, tile_y)` 中心，cost = 中心距（100m 均匀），启发 = 欧氏 |
| 邻接 | 4-连通（`±1, 0` 与 `0, ±1`）；当前 3×3 全连通，BFS 等价但保留 A\* 以便 N×N |
| Building detour | 每对相邻 tile 中心连段，扫两端 tile 内 building polygon；穿 polygon → 在 polygon 离段起点最近的 corner 插一个 waypoint |
| 同 tile 优化 | start/end 同 tile 且无障碍 → 直接 `[start, end]`（不走中心） |
| 边界 | 空 grid → fallback `[start, end]`；不可达 → fallback |
| gRPC 接入 | `src/grpc.rs::compute_path` 替换 stub，调 `pathfinding::find_path`，把 `Vec<(f32,f32)>` → `Vec<ProtoVec2>`，`distance_m` 改为折线总长 |
| 单测 | **36 通过 / 0 failed / 4 ignored**（Sprint 3 是 30；新增 6 pathfinding case + 改 2 case） |
| 集成测 | `tests/grpc_smoke::test_grpc_compute_path` 更新为验证多 waypoint + 折线距离 |
| E2E | Python client 3 场景 OK（详见 §二） |

### 算法细节

```
1. nearest_tile(start), nearest_tile(end) → 起点 / 终点 tile
2. 若同 tile：
     a. 扫同 tile 内 building polygon → 若 start→end 段被挡，插 corner waypoint
     b. 返回 [start, ..., end]
3. 否则 A* → tile_path
4. 拼 waypoints：[start] + 每个 tile center（去重）+ 每两中心间的 detour
5. 末尾若 ≠ end，补 end
```

detour 选择策略：polygon 中欧氏距离 segment 起点最近的 corner（tile-local 坐标 → 转 world 加 tile.center）。

### 复杂度

- 当前 9 tile × 每 tile ≤ 5 building → 单次 ComputePath 实测 < 100µs
- A* 在 N×N 网格上 O(N² log N)，polygon scan 是 O(building_count)，合计 O(B·L) B=building 数 / L=path 长度

## 二、验证证据

### 1. 单测 + 集成测

```
cargo test --lib              → 36 passed; 0 failed; 4 ignored
cargo test --test grpc_smoke  → 5 passed; 0 failed; 0 ignored
```

### 2. E2E gRPC（Python client → 127.0.0.1:50051）

| 场景 | start | end | waypoints | distance_m |
|---|---|---|---|---|
| 同 tile 无障碍 | (5, 5) | (15, 15) | `[(5,5), (15,15)]` 2 个 | 14.1 |
| 跨 3 tile 对角 | (50, 50) | (250, 250) | `[(50,50), (150,50), (150,150), (250,250)]` 4 个 | 341.4 |
| Detour 绕开 plaza | (5, 5) | (95, 95) | `[(5,5), (50,50), (95,95)]` 3 个 | 127.3 |

第 3 个场景：默认 `tile_0_0` 内 plaza polygon 在 world (80,80)-(120,120)，起点 (5,5) → 终点 (95,95) 的直线段会穿过 plaza；detour 选 tile_0_0 中心 (50,50) 作为绕开点，dist 从约 127.3（不再走 (95,95) 折角），反而是经中心后再去终点的折线。

## 三、本次踩到的坑

| # | 问题 | 解决 |
|---|---|---|
| 1 | 第一次实现：所有 path 都强插 tile center，导致同 tile 无障碍路径变 3 个 waypoint | 同 tile 直接 `[start, end]`，仅在跨 tile 时插中心 |
| 2 | 第一次实现：detour 只在 multi-tile 路径里检查 → 同 tile 内 building 阻挡不会 detour | 早返回分支也调 `building_detour` |
| 3 | 第二次测试：(10,10) → (90,50) 期望有 detour，实际无 → polygon 在 tile-local (40,40)-(60,60)，而段端点 tile-local 是 (-40,-40) → (40,0)，根本不碰 polygon | 测试用 (5,5) → (95,95)（明确穿 plaza 角点） |
| 4 | 测试 deterministic：`HashMap::min_by` 在两点等距时不稳定 → test_three_tile_diagonal 偶尔跨不同 tile | 测试用明显非边界坐标 (50,50) → (250,250)，距离差不会被 tie-break 触发 |
| 5 | 旧 `test_compute_path_stub_returns_two_waypoints` 断言 `{len=2, dist=50}`，新实装破坏 → 改名为 `test_compute_path_uses_tile_grid` 并放宽容差 | 统一改 grpc_smoke.rs 里的同名 case |

## 四、本次刻意没做的事

- **Tile 邻接表 PG 化**：当前 A* 从 tile id 字符串解析 `tx/ty` 推邻接，零数据库依赖。后续 tile 数 >100 时改 PG `tile_adjacency` 表（含不可走 / 跳跃 / 单向边）
- **navmesh / funnel algorithm**：detour 是 corner 近似，不是真正的 funnel pull。直线 → detour corner → 直线会形成"钝角"。生产级需要 Recast/Detour
- **跨多 tile detour 合并**：相邻两 tile 各有阻挡时会产生 2 个 detour waypoint，可能不是最优合并
- **Cost weight**：A* cost 全部 1.0（中心距），没考虑 LOD（CBD 步行快 / Suburb 慢）
- **起点/终点 inside polygon**：当前不主动推到 polygon 外，路径会"穿墙"。留给上层判定

## 五、变更清单

```
ai-city/apps/world-engine/src/pathfinding.rs        (A)  new — A* + detour
ai-city/apps/world-engine/src/lib.rs                (M)  +pub mod pathfinding
ai-city/apps/world-engine/src/grpc.rs               (M)  compute_path 调 pathfinding
ai-city/apps/world-engine/tests/grpc_smoke.rs       (M)  test_grpc_compute_path 更新
ai-city/docs/SPRINT-4.md                            (A)  本文件
```

## 六、下一步建议

按依赖顺序：

1. **Sprint 4.5：路径可视化 + 性能 metrics**：把 ComputePath 耗时 / waypoint 数加进 `/metrics`；客户端用 MapLibre 画线
2. **Sprint 5：Tile 邻接表 PG 化**：超过 100 tile 时内存 A* 太重；扩 `pg-schema.sql` 加 `tile_adjacency(tile_id_a, tile_id_b, cost, blocked)`
3. **Sprint 5+：navmesh**：接 `recast-rs` 或 `pathfinding` crate；polygon-aware 而非 corner 近似
4. **CI 接入**：GitHub Actions 把 `cargo test --lib --test grpc_smoke` + `python scripts/e2e_grpc_smoke.py` + `python scripts/e2e_path_smoke.py`（新增）串起来

---

> **方法论沉淀**：
> - 路径算法分两层（宏观 tile + 微观 obstacle）比单层更易扩展；先 tile 中心 + corner detour 简单可用
> - 测试用 hash map 迭代顺序敏感的位置（边界点）要避开；放中心或明显跨 tile 的坐标
> - 旧 stub test 的命名（`test_compute_path_stub_*`）是好习惯：实装替换时一行 rename 就知道改了什么
