//! World Engine - 城市物理与世界状态
//!
//! 职责：Tile 数据 / 碰撞 / 移动 / 多玩家同步
//! 详细设计见 docs/01-愿景.md §5 + docs/11-技术细节与玩法模式.md §B.1
//!
//! 性能目标：单实例支持 10,000 NPC + 1,000 玩家，TPS > 1000
//!
//! Sprint 3 状态：
//! - gRPC server（tonic）: 0.0.0.0:50051（WORLD_GRPC_ADDR）
//!   aicity.world.v1.WorldEngine = Move / GetTile / SubscribePosition / ComputePath
//! - REST API（axum）: 0.0.0.0:50052（WORLD_BIND_ADDR）
//!   /healthz /readyz /metrics /v1/tiles /v1/world/move /v1/players/:id/position
//! - Tile 来源：DATABASE_URL 已设置 → 手写 PG 客户端加载；否则 fallback `default_world()` 并 warn
//! - 内存 3×3 Tile 网格 + 玩家位置表 + Redis Pub/Sub 广播 player:moved
//! - SubscribePosition 从 REDIS_URL 订阅同一频道，按 tile_id 过滤后推流
//! - api-gateway 订阅该频道后写入 PG player_position
//!
//! 待办：
//! - api-gateway 接 gRPC client（Sprint 3.5）
//! - ComputePath 目前是直线 stub，A*/navmesh 留给 Sprint 4+

use std::net::SocketAddr;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Arc;

use anyhow::{Context as _, Result};
use tracing::{info, warn};
use tracing_subscriber::{layer::SubscriberExt, util::SubscriberInitExt};

use world_core::grpc::WorldEngineService;
use world_core::pg_client;
use world_core::rest::{serve, AppState};
use world_core::tile_loader;
use world_core::{RedisPub, RedisSub, WorldGrid};

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

    // Sprint 3: gRPC server 独立端口（proto: aicity.world.v1.WorldEngine）
    let grpc_addr: SocketAddr = std::env::var("WORLD_GRPC_ADDR")
        .unwrap_or_else(|_| "0.0.0.0:50051".to_string())
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

    // Sprint 3: Redis 订阅端（SubscribePosition 流的数据源）
    let redis_subscriber: Option<Arc<RedisSub>> = match std::env::var("REDIS_URL").ok() {
        Some(url) => {
            let sub = Arc::new(RedisSub::new(url, channel_moved.clone()));
            match sub.start().await {
                Ok(()) => {
                    info!(channel = %channel_moved, "redis subscriber started");
                    Some(sub)
                }
                Err(e) => {
                    warn!(error = ?e, "redis subscriber start failed, SubscribePosition disabled");
                    None
                }
            }
        }
        None => None,
    };

    let grpc_service = WorldEngineService {
        grid: grid.clone(),
        redis_pub: redis_client.clone(),
        redis_sub: redis_subscriber.clone(),
        channel_moved: channel_moved.clone(),
    };

    let state = AppState {
        grid,
        redis: redis_client,
        redis_sub: redis_subscriber,
        channel_moved,
        pg_connected,
        ready,
    };

    // 双 server 并行：gRPC (50051) + REST (50052)，任一退出即整体退出
    let grpc_task = tokio::spawn(async move {
        info!(addr = %grpc_addr, "gRPC server listening");
        tonic::transport::Server::builder()
            .add_service(grpc_service.into_server())
            .serve(grpc_addr)
            .await
            .context("grpc server")
    });

    let rest_task = tokio::spawn(async move { serve(bind_addr, state).await });

    tokio::select! {
        r = grpc_task => r.context("grpc task join")?,
        r = rest_task => r.context("rest task join")?,
    }
}

async fn load_from_pg(url: &str) -> Result<std::collections::HashMap<String, world_core::Tile>> {
    let params = pg_client::parse_pg_url(url).context("parse DATABASE_URL")?;
    let conn = pg_client::connect(&params)
        .await
        .with_context(|| format!("pg connect {}", params.host_port))?;
    let tiles = tile_loader::load_tiles(&conn).await.context("load_tiles")?;
    Ok(tiles)
}