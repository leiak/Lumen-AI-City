//! Sprint 2: 从 PG `tile` 表加载世界数据
//!
//! 设计：
//! - 启动时若 `DATABASE_URL` 已设置，调 `pg_client::connect` + `load_tiles`
//! - PG 不可达 / 超时 / 无 URL 时，调用方 fallback 到 `default_world()` 并 `warn!`
//! - 运行时仍由 `WorldGrid` 在内存中持有（高频读，不走 PG）
//! - `player_ids` 仅运行时维护，PG 不持久化（避免写放大）
//!
//! SQL 把所有列用 `::text` 转字符串、`buildings`/`npc_ids` 转 json 文本，
//! 在 Rust 侧 serde_json 解析 —— 避免手写 PG 二进制 row parser 的复杂度。

use std::collections::HashMap;

use anyhow::{Context as _, Result};
use serde::Deserialize;

use crate::pg_client::{self, PgConn};
use crate::tile::{Building, LodLevel, Tile};

const LOAD_TILES_SQL: &str = "SELECT id, \
    center_x::text AS center_x, \
    center_y::text AS center_y, \
    size::text AS size, \
    lod_level::text AS lod_level, \
    buildings::text AS buildings_json, \
    array_to_json(npc_ids)::text AS npc_ids_json \
    FROM tile WHERE enabled = TRUE ORDER BY id";

#[derive(Debug, Deserialize)]
struct TileRowStr {
    id: String,
    center_x: String,
    center_y: String,
    size: String,
    lod_level: String,
    buildings_json: String,
    npc_ids_json: String,
}

/// lod_level SMALLINT (0/1/2) → LodLevel
impl From<i16> for LodLevel {
    fn from(v: i16) -> Self {
        match v {
            0 => LodLevel::CBD,
            1 => LodLevel::Residential,
            _ => LodLevel::Suburb,
        }
    }
}

/// 加载所有 enabled tile → HashMap<id, Tile>
///
/// 失败：DB 不可达 / SQL 错 / JSON 反序列化错 → anyhow 错误
pub async fn load_tiles(conn: &PgConn) -> Result<HashMap<String, Tile>> {
    let raw_rows = pg_client::query_simple(conn, LOAD_TILES_SQL).await?;
    let mut grid = HashMap::with_capacity(raw_rows.len());

    for row in raw_rows {
        if row.len() != 7 {
            anyhow::bail!("tile row expects 7 columns, got {}", row.len());
        }
        let mut s = TileRowStr {
            id: row[0].clone().context("tile.id NULL")?,
            center_x: row[1].clone().context("tile.center_x NULL")?,
            center_y: row[2].clone().context("tile.center_y NULL")?,
            size: row[3].clone().context("tile.size NULL")?,
            lod_level: row[4].clone().context("tile.lod_level NULL")?,
            buildings_json: row[5].clone().context("tile.buildings NULL")?,
            npc_ids_json: row[6].clone().context("tile.npc_ids NULL")?,
        };

        let center_x: f32 = s.center_x.parse().context("center_x parse")?;
        let center_y: f32 = s.center_y.parse().context("center_y parse")?;
        let size: f32 = s.size.parse().context("size parse")?;
        let lod_i: i16 = s.lod_level.parse().context("lod_level parse")?;
        let lod: LodLevel = lod_i.into();
        let buildings: Vec<Building> =
            serde_json::from_str(&s.buildings_json).context("buildings JSON parse")?;
        let npc_ids: Vec<String> =
            serde_json::from_str(&s.npc_ids_json).context("npc_ids JSON parse")?;
        // 防借用：把 s.id 移出来后再用
        let id = std::mem::take(&mut s.id);

        let tile = Tile {
            id: id.clone(),
            center_x,
            center_y,
            size,
            buildings,
            npc_ids,
            player_ids: Vec::new(),
            lod_level: lod,
        };
        grid.insert(id, tile);
    }
    Ok(grid)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_lod_level_from_i16() {
        assert!(matches!(LodLevel::from(0i16), LodLevel::CBD));
        assert!(matches!(LodLevel::from(1i16), LodLevel::Residential));
        assert!(matches!(LodLevel::from(2i16), LodLevel::Suburb));
        // 越界降级到 Suburb（不 panic）
        assert!(matches!(LodLevel::from(99i16), LodLevel::Suburb));
        assert!(matches!(LodLevel::from(-1i16), LodLevel::Suburb));
    }

    /// 集成测试：需要本机 aicity-pg 在 127.0.0.1:5432 可达且 schema 已 apply。
    /// `cargo test -- --ignored` 才跑。
    #[tokio::test]
    #[ignore = "requires local aicity-pg on 127.0.0.1:5432 with tile table"]
    async fn test_load_tiles_roundtrip() {
        let params = pg_client::parse_pg_url("postgres://aicity@127.0.0.1:5432/aicity").unwrap();
        let conn = pg_client::connect(&params).await.expect("connect");
        let tiles = load_tiles(&conn).await.expect("load");
        assert!(tiles.len() >= 9, "expected >= 9 tiles, got {}", tiles.len());
        let cbd = tiles.get("tile_0_0").expect("tile_0_0 missing");
        assert!(matches!(cbd.lod_level, LodLevel::CBD));
        assert!(cbd.buildings.len() >= 2);
        assert!(cbd.npc_ids.contains(&"npc_wang_boss_001".to_string()));
    }
}