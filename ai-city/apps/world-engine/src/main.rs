//! World Engine - 城市物理与世界状态
//!
//! 职责：Tile 数据 / 碰撞 / 移动 / 多玩家同步
//! 详细设计见 docs/01-愿景.md §5 + docs/11-技术细节与玩法模式.md §B.1
//!
//! 性能目标：单实例支持 10,000 NPC + 1,000 玩家，TPS > 1000
//!
//! Sprint 2 状态：
//! - REST API（axum）: /healthz /readyz /metrics /v1/tiles /v1/world/move /v1/players/:id/position
//! - Tile 来源：DATABASE_URL 已设置 → 手写 PG 客户端加载；-- 否则 fallback `default_world()` 并 warn
//! - 内存 3×3 Tile 网格 + 玩家位置表 + Redis Pub/Sub 广播 player:moved
//! - api-gateway 订阅该频道后写入 PG player_position
//!
//! 待办：
//! - gRPC server (tonic)：proto 已就位，main.rs 待接入
//! - 多实例 Redis Pub/Sub 完整覆盖

use std::net::SocketAddr;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Arc;

use anyhow::{Context as _, Result};
use tracing::{info, warn};
use tracing_subscriber::{layer::SubscriberExt, util::SubscriberInitExt};

use world_core::pg_client;
use world_core::rest::{serve, AppState};
use world_core::tile_loader;
use world_core::{RedisPub, WorldGrid};

#[tokio::main]
async fn main() -> Result<()> {
    tracing_subscriber::registry()
        .with(tracing_subscriber::EnvFilter::new(
            std::env::var("RUST_LOG").unwrap_or_else(|_| "info".into()),
        ))
        .with(tracing_subscriber::fmt::layer())
        .init();

    info!("world-engine starting");

    let bind_addr: SocketAddr = std::env::var("WORLD_BIND_ADDR")
        .unwrap_or_else(|_| "0.0.0.0:50052".to_string())
        .parse()?;

    let redis_client: Option<Arc<RedisPub>> = std::env::var("REDIS_URL").ok().map(|url| {
        info!(redis_url = %url, "redis publisher ready");
        Arc::new(RedisPub::new(url))
    });

    let channel_moved = std::env::var("REDIS_CHANNEL_MOVED")
        .unwrap_or_else(|_| "aicity:player:moved".to_string());

    // Sprint 2: 启动时尝试 PG 连接 + 加载 tile，失败 fallback
    let pg_connected = Arc::new(AtomicBool::new(false));
    let grid = match std::env::var("DATABASE_URL").ok() {
        Some(url) => {
            info!(pg_url = %url, "connecting to postgres…");
            match load_from_pg(&url).await {
                Ok(tiles) => {
                    let n = tiles.len();
                    pg_connected.store(true, Ordering::Relaxed);
                    info!(tiles_loaded = n, "tile grid loaded from pg");
                    Arc::new(WorldGrid::with_tiles(tiles))
                }
                Err(e) => {
                    warn!(error = ?e, "pg load failed, fallback default_world()");
                    Arc::new(WorldGrid::new())
                }
            }
        }
        None => {
            info!("DATABASE_URL not set, using default_world()");
            Arc::new(WorldGrid::new())
        }
    };

    let ready = Arc::new(AtomicBool::new(true));

    let state = AppState {
        grid,
        redis: redis_client,
        channel_moved,
        pg_connected,
        ready,
    };

    serve(bind_addr, state).await
}

async fn load_from_pg(url: &str) -> Result<std::collections::HashMap<String, world_core::Tile>> {
    let params = pg_client::parse_pg_url(url).context("parse DATABASE_URL")?;
    let conn = pg_client::connect(&params)
        .await
        .with_context(|| format!("pg connect {}", params.host_port))?;
    let tiles = tile_loader::load_tiles(&conn).await.context("load_tiles")?;
    Ok(tiles)
}