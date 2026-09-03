//! Tile 数据结构
//! 城市被划分为正方形 Tile（默认 100m × 100m）

use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Tile {
    pub id: String,
    pub center_x: f32,
    pub center_y: f32,
    pub size: f32, // 默认 100.0
    pub buildings: Vec<Building>,
    pub npc_ids: Vec<String>,
    pub player_ids: Vec<String>,
    pub lod_level: LodLevel,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum LodLevel {
    /// 主城区：高密度
    CBD,
    /// 居民区：中密度
    Residential,
    /// 郊区：低密度
    Suburb,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Building {
    pub id: String,
    pub kind: BuildingKind,
    pub polygon: Vec<(f32, f32)>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum BuildingKind {
    Tavern,
    Shop,
    House,
    Park,
    Road,
    Plaza,
    Office,
}

impl Tile {
    /// 根据坐标计算所属 Tile
    pub fn from_xy(x: f32, y: f32, size: f32) -> String {
        let tx = (x / size).floor() as i32;
        let ty = (y / size).floor() as i32;
        format!("tile_{}_{}", tx, ty)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_tile_id() {
        assert_eq!(Tile::from_xy(0.0, 0.0, 100.0), "tile_0_0");
        assert_eq!(Tile::from_xy(150.0, 250.0, 100.0), "tile_1_2");
        assert_eq!(Tile::from_xy(-50.0, -150.0, 100.0), "tile_-1_-2");
    }
}
