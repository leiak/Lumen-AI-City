//! Sprint 4: 真路径（ComputePath 实装）
//!
//! 算法两层：
//! 1. **Tile 级 A\***：在 (tile_x, tile_y) 网格上做 A*，节点 = tile 中心，cost = 中心距。
//!    对当前 9 tile 3×3 世界退化成 BFS（cost uniform），但保留 A* 以便后续扩到 N×N。
//! 2. **Building detour**：每两个相邻 tile 中心连一条直线段，扫描穿越的两个 tile 里
//!    所有 building polygon，若线段穿过某 polygon → 在该 polygon 离段起点最近的
//!    corner 插一个 waypoint 绕开（简单但足够 demo）。
//!
//! 边界：
//! - start / end 在同一 tile → 直接两 waypoint（除非被 building 挡，则 detour）
//! - start 或 end 在 building polygon 内 → 仍然返回路径，但 tile 级从最近 tile 出发
//!   （不主动推到 polygon 外；留给上层判定"卡墙里"）
//! - 不可达：返回 [start, end]（fallback 到直线）
//!
//! 性能预算：
//! - 当前 9 tile × 每 tile ≤5 building → 单次 ComputePath ≤ 1ms（hash lookup 主导）

use std::collections::{BinaryHeap, HashMap};

use crate::tile::Tile;
use crate::world_grid::WorldGrid;

/// 2D 坐标（世界坐标，米）
pub type Pt = (f32, f32);

/// 主入口：在 WorldGrid 上找 start → end 的 waypoint 序列（含起点终点）
///
/// 返回的 Vec 第一个元素 = start，最后一个 = end，中间是 tile 中心 + 可选 detour。
pub fn find_path(grid: &WorldGrid, start: Pt, end: Pt) -> Vec<Pt> {
    let tiles = grid.list();
    if tiles.is_empty() {
        return vec![start, end];
    }

    // 0) 同 tile 且无障碍 → 直接两 waypoint（不走 tile 中心，性能 + 美观）
    let start_tile = nearest_tile(&tiles, start);
    let end_tile = nearest_tile(&tiles, end);
    if start_tile == end_tile {
        // 仍然检查 start→end 直线是否被同 tile 内 building 挡
        if let Some(detour) = building_detour(&tiles, &start_tile, &start_tile, start, end) {
            return vec![start, detour, end];
        }
        return vec![start, end];
    }

    // 1) tile 级 A*（用 center 当节点）
    let tile_path: Vec<String> = match astar_tile(&tiles, &start_tile, &end_tile) {
        Some(p) => p,
        None => return vec![start, end], // 不可达 fallback
    };

    // 2) 把 tile 中心拼成 waypoints（多 tile 才有中间节点）
    let center_map: HashMap<&str, Pt> = tiles
        .iter()
        .map(|t| (t.id.as_str(), (t.center_x, t.center_y)))
        .collect();

    let mut waypoints: Vec<Pt> = Vec::with_capacity(tile_path.len() + 4);
    waypoints.push(start);
    for (i, tid) in tile_path.iter().enumerate() {
        let c = center_map[tid.as_str()];
        // 跳过与上一点重合的（首 tile 中心可能 = start，末 tile = end）
        if waypoints.last().map_or(true, |p| approx_dist(*p, c) > 1e-3) {
            waypoints.push(c);
        }
        // 当前 → 下一 tile 中心连线被 building 挡 → detour
        if i + 1 < tile_path.len() {
            let next_tid = &tile_path[i + 1];
            let next_c = center_map[next_tid.as_str()];
            if let Some(detour) = building_detour(&tiles, tid, next_tid, c, next_c) {
                if detour != c && detour != next_c {
                    waypoints.push(detour);
                }
            }
        }
    }
    if waypoints.last().map_or(true, |p| approx_dist(*p, end) > 1e-3) {
        waypoints.push(end);
    }

    waypoints
}

// ─── Tile 级 A* ──────────────────────────────────────────────────────────

/// 在 4-连通 tile 网格上 A*（曼哈顿启发 + 实际 cost = 中心距）
fn astar_tile(tiles: &[Tile], start: &str, end: &str) -> Option<Vec<String>> {
    let index: HashMap<&str, &Tile> = tiles.iter().map(|t| (t.id.as_str(), t)).collect();

    let s = *index.get(start)?;
    let e = *index.get(end)?;
    let heur = |t: &Tile| euclid((t.center_x, t.center_y), (e.center_x, e.center_y));

    #[derive(Copy, Clone, PartialEq)]
    struct Node {
        f: f32,
        idx: usize,
    }
    // min-heap by f (BinaryHeap is max-heap → 反转 f 的符号)
    impl Eq for Node {}
    impl Ord for Node {
        fn cmp(&self, o: &Self) -> std::cmp::Ordering {
            self.f.partial_cmp(&o.f).unwrap_or(std::cmp::Ordering::Equal).reverse()
        }
    }
    impl PartialOrd for Node {
        fn partial_cmp(&self, o: &Self) -> Option<std::cmp::Ordering> {
            Some(self.cmp(o))
        }
    }

    let pos: HashMap<&str, usize> = tiles.iter().enumerate().map(|(i, t)| (t.id.as_str(), i)).collect();
    let s_idx = *pos.get(start)?;
    let e_idx = *pos.get(end)?;

    let mut g: HashMap<usize, f32> = HashMap::new();
    let mut came: HashMap<usize, usize> = HashMap::new();
    let mut open: BinaryHeap<Node> = BinaryHeap::new();

    g.insert(s_idx, 0.0);
    open.push(Node { f: heur(s), idx: s_idx });

    while let Some(Node { idx, .. }) = open.pop() {
        if idx == e_idx {
            // reconstruct
            let mut path = vec![e_idx];
            let mut cur = e_idx;
            while let Some(&p) = came.get(&cur) {
                path.push(p);
                cur = p;
            }
            path.reverse();
            return Some(path.iter().map(|&i| tiles[i].id.clone()).collect());
        }
        let cur_tile = &tiles[idx];
        for nb in neighbors(cur_tile, &index) {
            let nb_idx = *pos.get(nb.id.as_str()).unwrap();
            let step = euclid((cur_tile.center_x, cur_tile.center_y), (nb.center_x, nb.center_y));
            let tentative = g[&idx] + step;
            if tentative < *g.get(&nb_idx).unwrap_or(&f32::INFINITY) {
                came.insert(nb_idx, idx);
                g.insert(nb_idx, tentative);
                open.push(Node { f: tentative + heur(nb), idx: nb_idx });
            }
        }
    }
    None
}

fn neighbors<'a>(t: &Tile, idx: &'a HashMap<&str, &'a Tile>) -> Vec<&'a Tile> {
    let (tx, ty) = parse_tile_id(&t.id).unwrap_or((0, 0));
    [(1, 0), (-1, 0), (0, 1), (0, -1)]
        .iter()
        .filter_map(|(dx, dy)| {
            let nid = format!("tile_{}_{}", tx + dx, ty + dy);
            idx.get(nid.as_str()).copied()
        })
        .collect()
}

fn parse_tile_id(id: &str) -> Option<(i32, i32)> {
    let s = id.strip_prefix("tile_")?;
    let (x, y) = s.split_once('_')?;
    Some((x.parse().ok()?, y.parse().ok()?))
}

fn nearest_tile(tiles: &[Tile], p: Pt) -> String {
    tiles
        .iter()
        .min_by(|a, b| {
            euclid((a.center_x, a.center_y), p)
                .partial_cmp(&euclid((b.center_x, b.center_y), p))
                .unwrap_or(std::cmp::Ordering::Equal)
        })
        .map(|t| t.id.clone())
        .unwrap_or_default()
}

// ─── Building detour ─────────────────────────────────────────────────────

/// 检查 segment [from, to] 是否穿过 tile_from 或 tile_to 里任何 building polygon。
/// 若穿过 → 返回绕开点的世界坐标（取 polygon 离 from 最近的 corner + tile center）。
fn building_detour(
    tiles: &[Tile],
    tile_from_id: &str,
    tile_to_id: &str,
    from: Pt,
    to: Pt,
) -> Option<Pt> {
    let tmap: HashMap<&str, &Tile> = tiles.iter().map(|t| (t.id.as_str(), t)).collect();
    for tid in [tile_from_id, tile_to_id] {
        let tile = match tmap.get(tid) {
            Some(t) => *t,
            None => continue,
        };
        for b in &tile.buildings {
            // 把 segment 转到 tile-local（polygon 在 tile-local 系下）
            let local_from = (from.0 - tile.center_x, from.1 - tile.center_y);
            let local_to = (to.0 - tile.center_x, to.1 - tile.center_y);
            if segment_intersects_polygon(local_from, local_to, &b.polygon) {
                // 找 polygon 中离 local_from 最近的 corner
                let mut best: Option<Pt> = None;
                let mut best_d = f32::INFINITY;
                for &p in &b.polygon {
                    let d = euclid(p, local_from);
                    if d < best_d {
                        best_d = d;
                        best = Some(p);
                    }
                }
                if let Some(p) = best {
                    return Some((p.0 + tile.center_x, p.1 + tile.center_y));
                }
            }
        }
    }
    None
}

/// 线段 p1-p2 与多边形 poly（凸，vertices 顺/逆序皆可）是否相交
///
/// 算法：遍历每条边 (poly[i], poly[(i+1)%n])，判断 segment-segment 是否相交。
/// 简单 robust 实现（用 orientation + on_segment）。
fn segment_intersects_polygon(p1: Pt, p2: Pt, poly: &[Pt]) -> bool {
    let n = poly.len();
    if n < 3 {
        return false;
    }
    for i in 0..n {
        let q1 = poly[i];
        let q2 = poly[(i + 1) % n];
        if segments_intersect(p1, p2, q1, q2) {
            return true;
        }
    }
    false
}

fn segments_intersect(p1: Pt, p2: Pt, q1: Pt, q2: Pt) -> bool {
    let o1 = orientation(p1, p2, q1);
    let o2 = orientation(p1, p2, q2);
    let o3 = orientation(q1, q2, p1);
    let o4 = orientation(q1, q2, p2);
    o1 != o2 && o3 != o4
}

fn orientation(a: Pt, b: Pt, c: Pt) -> i8 {
    let v = (b.0 - a.0) * (c.1 - a.1) - (b.1 - a.1) * (c.0 - a.0);
    if v > 1e-6 {
        1
    } else if v < -1e-6 {
        -1
    } else {
        0
    }
}

// ─── helpers ─────────────────────────────────────────────────────────────

fn euclid(a: Pt, b: Pt) -> f32 {
    let dx = a.0 - b.0;
    let dy = a.1 - b.1;
    (dx * dx + dy * dy).sqrt()
}

fn approx_dist(a: Pt, b: Pt) -> f32 {
    let dx = a.0 - b.0;
    let dy = a.1 - b.1;
    (dx * dx + dy * dy).sqrt()
}

// ─── Tests ───────────────────────────────────────────────────────────────

#[cfg(test)]
mod tests {
    use super::*;
    use crate::tile::{Building, BuildingKind, LodLevel};

    fn grid_with_blocking_building() -> WorldGrid {
        // 单 tile (tile_0_0) with a building blocking center→(100,50)
        let mut tiles = HashMap::new();
        tiles.insert(
            "tile_0_0".to_string(),
            Tile {
                id: "tile_0_0".into(),
                center_x: 50.0,
                center_y: 50.0,
                size: 100.0,
                buildings: vec![Building {
                    id: "wall".into(),
                    kind: BuildingKind::House,
                    // 矩形覆盖 (40,40)-(60,60) → 阻挡 tile 中心到 (100,50) 的直连线
                    polygon: vec![(40.0, 40.0), (60.0, 40.0), (60.0, 60.0), (40.0, 60.0)],
                }],
                npc_ids: vec![],
                player_ids: vec![],
                lod_level: LodLevel::CBD,
            },
        );
        WorldGrid::with_tiles(tiles)
    }

    #[test]
    fn test_same_tile_no_obstacle_returns_two_points() {
        let grid = WorldGrid::new(); // default 9 tile + 0 obstacle on path
        let path = find_path(&grid, (5.0, 5.0), (15.0, 15.0));
        // start, end（同一 tile，无 detour）
        assert_eq!(path.len(), 2);
        assert_eq!(path[0], (5.0, 5.0));
        assert_eq!(path[1], (15.0, 15.0));
    }

    #[test]
    fn test_cross_tile_path_returns_tile_centers() {
        let grid = WorldGrid::new();
        // tile_0_0 (50,50) → tile_1_0 (150,50)
        let path = find_path(&grid, (50.0, 50.0), (150.0, 50.0));
        // 至少 3 个 waypoint（start, tile_0_0 center, tile_1_0 center, end）
        // 但 start == tile_0_0 center 且 end == tile_1_0 center → 实际 2 个
        assert!(path.len() >= 2, "got {:?}", path);
        assert_eq!(path[0], (50.0, 50.0));
        assert_eq!(path[path.len() - 1], (150.0, 50.0));
    }

    #[test]
    fn test_detour_around_blocking_building() {
        let grid = grid_with_blocking_building();
        // start (5,5) → end (95,95) 都在 tile_0_0 内
        // 直线段穿过 polygon (40,40)-(60,60)（世界 (90,90)-(110,110)）→ 应插 1 个 detour waypoint
        let path = find_path(&grid, (5.0, 5.0), (95.0, 95.0));
        assert!(path.len() >= 3, "expected detour, got {:?}", path);
        let middle = path[1];
        assert_ne!(middle, (5.0, 5.0));
        assert_ne!(middle, (95.0, 95.0));
    }

    #[test]
    fn test_empty_grid_returns_start_end() {
        let grid = WorldGrid::with_tiles(HashMap::new());
        let path = find_path(&grid, (0.0, 0.0), (10.0, 10.0));
        assert_eq!(path, vec![(0.0, 0.0), (10.0, 10.0)]);
    }

    #[test]
    fn test_three_tile_diagonal_path() {
        let grid = WorldGrid::new();
        // tile_0_0 (50,50) → tile_2_2 (250,250) — 走对角
        let path = find_path(&grid, (50.0, 50.0), (250.0, 250.0));
        assert!(path.len() >= 4, "expected multi-tile path, got {:?}", path);
        assert_eq!(path[0], (50.0, 50.0));
        assert_eq!(path[path.len() - 1], (250.0, 250.0));
        // waypoint 中应包含 tile_1_1 center (150,150)
        assert!(
            path.iter().any(|&p| approx_dist(p, (150.0, 150.0)) < 1.0),
            "expected to pass through tile_1_1, got {:?}",
            path
        );
    }

    #[test]
    fn test_segment_polygon_basic() {
        // 直线穿过矩形
        let poly = vec![(0.0, 0.0), (10.0, 0.0), (10.0, 10.0), (0.0, 10.0)];
        assert!(segment_intersects_polygon((-5.0, 5.0), (15.0, 5.0), &poly));
        // 平行但不穿过
        assert!(!segment_intersects_polygon((-5.0, 20.0), (15.0, 20.0), &poly));
        // 完全在矩形内
        assert!(!segment_intersects_polygon((2.0, 2.0), (8.0, 8.0), &poly));
    }
}
