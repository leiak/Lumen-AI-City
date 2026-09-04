//! 内存中的世界网格（Sprint 1 简化版）
//!
//! 默认加载一个 3×3 的 Tile 网格（中心在 tile_0_0）。
//! 后续 Sprint 会接入 PG `tile` 表 + Redis Pub/Sub 做分布式加载。
//!
//! 坐标系：
//! - Tile 大小 100m × 100m
//! - tile_x_y 表示坐标范围 [x*100, (x+1)*100) × [y*100, (y+1)*100)
//!
//! Sprint 1.5 新增：
//! - `PlayerPosition`：每个玩家的实时位置（X/Y/Tile/时间戳）
//! - `player_enter` / `player_leave` 维护 tile.player_ids 集合

use std::collections::HashMap;
use std::sync::RwLock;

use serde::{Deserialize, Serialize};

use crate::tile::{Building, BuildingKind, LodLevel, Tile};

const TILE_SIZE: f32 = 100.0;
const GRID_RADIUS: i32 = 1; // 中心 ±1 → 3×3 = 9 个 Tile

/// 玩家实时位置
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PlayerPosition {
    pub player_id: String,
    pub tile_id: String,
    pub x: f32,
    pub y: f32,
    pub ts_ms: i64,
}

/// 默认世界：3×3 网格 + 3 个种子 NPC 所在的 Tile
pub fn default_world() -> HashMap<String, Tile> {
    let mut grid = HashMap::new();

    for tx in -GRID_RADIUS..=GRID_RADIUS {
        for ty in -GRID_RADIUS..=GRID_RADIUS {
            let id = format!("tile_{}_{}", tx, ty);
            let lod = match (tx, ty) {
                (0, 0) => LodLevel::CBD,
                (0, _) | (_, 0) => LodLevel::Residential,
                _ => LodLevel::Suburb,
            };
            let mut npc_ids = Vec::new();
            let buildings = match (tx, ty) {
                (0, 0) => {
                    npc_ids.push("npc_wang_boss_001".to_string());
                    vec![
                        Building {
                            id: "bldg_tavern_0_0".into(),
                            kind: BuildingKind::Tavern,
                            polygon: vec![(0.0, 0.0), (20.0, 0.0), (20.0, 15.0), (0.0, 15.0)],
                        },
                        Building {
                            id: "bldg_plaza_0_0".into(),
                            kind: BuildingKind::Plaza,
                            polygon: vec![(30.0, 30.0), (70.0, 30.0), (70.0, 70.0), (30.0, 70.0)],
                        },
                    ]
                },
                (1, 0) => {
                    npc_ids.push("npc_lihua_001".to_string());
                    vec![
                        Building {
                            id: "bldg_house_1_0".into(),
                            kind: BuildingKind::House,
                            polygon: vec![(10.0, 10.0), (25.0, 10.0), (25.0, 25.0), (10.0, 25.0)],
                        },
                        Building {
                            id: "bldg_shop_1_0".into(),
                            kind: BuildingKind::Shop,
                            polygon: vec![(60.0, 60.0), (80.0, 60.0), (80.0, 80.0), (60.0, 80.0)],
                        },
                    ]
                },
                (-1, 1) => {
                    npc_ids.push("npc_zhang_granny_001".to_string());
                    vec![
                        Building {
                            id: "bldg_park_-1_1".into(),
                            kind: BuildingKind::Park,
                            polygon: vec![(0.0, 0.0), (90.0, 0.0), (90.0, 90.0), (0.0, 90.0)],
                        },
                    ]
                },
                _ => Vec::new(),
            };

            let tile = Tile {
                id: id.clone(),
                center_x: tx as f32 * TILE_SIZE + TILE_SIZE / 2.0,
                center_y: ty as f32 * TILE_SIZE + TILE_SIZE / 2.0,
                size: TILE_SIZE,
                buildings,
                npc_ids,
                player_ids: Vec::new(),
                lod_level: lod,
            };
            grid.insert(id, tile);
        }
    }

    grid
}

/// 线程安全的网格容器 + 玩家位置表
pub struct WorldGrid {
    inner: RwLock<HashMap<String, Tile>>,
    positions: RwLock<HashMap<String, PlayerPosition>>,
}

impl WorldGrid {
    pub fn new() -> Self {
        Self {
            inner: RwLock::new(default_world()),
            positions: RwLock::new(HashMap::new()),
        }
    }

    pub fn get(&self, id: &str) -> Option<Tile> {
        self.inner.read().ok()?.get(id).cloned()
    }

    pub fn list(&self) -> Vec<Tile> {
        self.inner.read().map(|g| g.values().cloned().collect()).unwrap_or_default()
    }

    /// 玩家进入 Tile，更新其 player_ids 列表
    pub fn player_enter(&self, tile_id: &str, player_id: &str) -> bool {
        if let Ok(mut g) = self.inner.write() {
            if let Some(t) = g.get_mut(tile_id) {
                if !t.player_ids.iter().any(|p| p == player_id) {
                    t.player_ids.push(player_id.to_string());
                }
                return true;
            }
        }
        false
    }

    /// 玩家离开 Tile
    pub fn player_leave(&self, tile_id: &str, player_id: &str) {
        if let Ok(mut g) = self.inner.write() {
            if let Some(t) = g.get_mut(tile_id) {
                t.player_ids.retain(|p| p != player_id);
            }
        }
    }

    /// 更新玩家位置（覆盖式）
    pub fn upsert_position(&self, pos: PlayerPosition) {
        // 同步更新 Tile.player_ids
        if let Some(prev) = self.get_position(&pos.player_id) {
            if prev.tile_id != pos.tile_id {
                self.player_leave(&prev.tile_id, &pos.player_id);
                self.player_enter(&pos.tile_id, &pos.player_id);
            }
        } else {
            self.player_enter(&pos.tile_id, &pos.player_id);
        }
        if let Ok(mut m) = self.positions.write() {
            m.insert(pos.player_id.clone(), pos);
        }
    }

    pub fn get_position(&self, player_id: &str) -> Option<PlayerPosition> {
        self.positions.read().ok()?.get(player_id).cloned()
    }

    /// 当前 Tile 上所有玩家位置
    pub fn positions_in_tile(&self, tile_id: &str) -> Vec<PlayerPosition> {
        self.positions
            .read()
            .map(|m| m.values().filter(|p| p.tile_id == tile_id).cloned().collect())
            .unwrap_or_default()
    }

    /// 当前在线玩家数（metrics 用）
    pub fn player_count(&self) -> usize {
        self.positions.read().map(|m| m.len()).unwrap_or(0)
    }
}

impl Default for WorldGrid {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_default_world_has_9_tiles() {
        let g = default_world();
        assert_eq!(g.len(), 9);
        assert!(g.contains_key("tile_0_0"));
        assert!(g.contains_key("tile_1_0"));
        assert!(g.contains_key("tile_-1_1"));
    }

    #[test]
    fn test_seeded_npcs_in_tiles() {
        let g = default_world();
        let t0 = g.get("tile_0_0").unwrap();
        assert!(t0.npc_ids.contains(&"npc_wang_boss_001".to_string()));
    }

    #[test]
    fn test_player_enter_leave() {
        let grid = WorldGrid::new();
        assert!(grid.player_enter("tile_0_0", "player_demo"));
        let t = grid.get("tile_0_0").unwrap();
        assert!(t.player_ids.contains(&"player_demo".to_string()));

        grid.player_leave("tile_0_0", "player_demo");
        let t = grid.get("tile_0_0").unwrap();
        assert!(!t.player_ids.contains(&"player_demo".to_string()));
    }

    #[test]
    fn test_upsert_position_updates_tile_membership() {
        let grid = WorldGrid::new();
        let now = chrono_now_ms();

        grid.upsert_position(PlayerPosition {
            player_id: "p1".into(),
            tile_id: "tile_0_0".into(),
            x: 50.0,
            y: 50.0,
            ts_ms: now,
        });
        assert!(grid.get("tile_0_0").unwrap().player_ids.contains(&"p1".to_string()));

        // 移动到 tile_1_0
        grid.upsert_position(PlayerPosition {
            player_id: "p1".into(),
            tile_id: "tile_1_0".into(),
            x: 150.0,
            y: 50.0,
            ts_ms: now + 100,
        });
        assert!(!grid.get("tile_0_0").unwrap().player_ids.contains(&"p1".to_string()));
        assert!(grid.get("tile_1_0").unwrap().player_ids.contains(&"p1".to_string()));
    }

    #[test]
    fn test_positions_in_tile() {
        let grid = WorldGrid::new();
        let now = chrono_now_ms();
        grid.upsert_position(PlayerPosition {
            player_id: "p1".into(),
            tile_id: "tile_0_0".into(),
            x: 50.0,
            y: 50.0,
            ts_ms: now,
        });
        grid.upsert_position(PlayerPosition {
            player_id: "p2".into(),
            tile_id: "tile_1_0".into(),
            x: 150.0,
            y: 50.0,
            ts_ms: now,
        });
        let in_0_0 = grid.positions_in_tile("tile_0_0");
        assert_eq!(in_0_0.len(), 1);
        assert_eq!(in_0_0[0].player_id, "p1");
    }

    fn chrono_now_ms() -> i64 {
        std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .map(|d| d.as_millis() as i64)
            .unwrap_or(0)
    }
}