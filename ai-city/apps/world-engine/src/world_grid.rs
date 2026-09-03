//! 内存中的世界网格（Sprint 1 简化版）
//!
//! 默认加载一个 3×3 的 Tile 网格（中心在 tile_0_0）。
//! 后续 Sprint 会接入 PG `tile` 表 + Redis Pub/Sub 做分布式加载。
//!
//! 坐标系：
//! - Tile 大小 100m × 100m
//! - tile_x_y 表示坐标范围 [x*100, (x+1)*100) × [y*100, (y+1)*100)

use std::collections::HashMap;
use std::sync::RwLock;

use crate::tile::{Building, BuildingKind, LodLevel, Tile};

const TILE_SIZE: f32 = 100.0;
const GRID_RADIUS: i32 = 1; // 中心 ±1 → 3×3 = 9 个 Tile

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

/// 线程安全的网格容器
pub struct WorldGrid {
    inner: RwLock<HashMap<String, Tile>>,
}

impl WorldGrid {
    pub fn new() -> Self {
        Self {
            inner: RwLock::new(default_world()),
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
}