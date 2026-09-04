//! World Engine - 城市物理与世界状态
//!
//! 职责：Tile 数据 / 碰撞 / 移动 / 多玩家同步
//! 详细设计见 docs/01-愿景.md §5 + docs/11-技术细节与玩法模式.md §B.1
//!
//! 性能目标：单实例支持 10,000 NPC + 1,000 玩家，TPS > 1000
//!
//! Sprint 1.5 状态：
//! - gRPC stub（tonic）占位
//! - REST API（axum）: /healthz, /v1/tiles, /v1/world/move, /v1/players/:id/position
//! - 内存 3×3 Tile 网格 + 玩家位置表 + Redis Pub/Sub 广播 player:moved
//! - api-gateway 订阅该频道后写入 PG player_position

use std::net::SocketAddr;
use std::sync::Arc;

use anyhow::Result;
use tracing_subscriber::{layer::SubscriberExt, util::SubscriberInitExt};

use world_core::rest::{serve, AppState};
use world_core::{RedisPub, WorldGrid};

#[tokio::main]
async fn main() -> Result<()> {
    tracing_subscriber::registry()
        .with(tracing_subscriber::EnvFilter::new(
            std::env::var("RUST_LOG").unwrap_or_else(|_| "info".into()),
        ))
        .with(tracing_subscriber::fmt::layer())
        .init();

    tracing::info!("world-engine starting");

    let bind_addr: SocketAddr = std::env::var("WORLD_BIND_ADDR")
        .unwrap_or_else(|_| "0.0.0.0:50052".to_string())
        .parse()?;

    let redis_client: Option<Arc<RedisPub>> = std::env::var("REDIS_URL").ok().map(|url| {
        tracing::info!(redis_url = %url, "redis publisher ready");
        Arc::new(RedisPub::new(url))
    });

    let channel_moved = std::env::var("REDIS_CHANNEL_MOVED")
        .unwrap_or_else(|_| "aicity:player:moved".to_string());

    let state = AppState {
        grid: Arc::new(WorldGrid::new()),
        redis: redis_client,
        channel_moved,
    };

    // TODO: gRPC server (tonic) — Sprint 2 接入
    // TODO: PG tile 持久化加载
    // TODO: 多实例 Redis Pub/Sub 完整覆盖（移动/位置订阅端点）

    serve(bind_addr, state).await
}