//! World Engine - 城市物理与世界状态
//!
//! 职责：Tile 数据 / 碰撞 / 移动 / 多玩家同步
//! 详细设计见 docs/01-愿景.md §5 + docs/11-技术细节与玩法模式.md §B.1
//!
//! 性能目标：单实例支持 10,000 NPC + 1,000 玩家，TPS > 1000

use anyhow::Result;
use tracing_subscriber::{layer::SubscriberExt, util::SubscriberInitExt};

#[tokio::main]
async fn main() -> Result<()> {
    tracing_subscriber::registry()
        .with(tracing_subscriber::EnvFilter::new(
            std::env::var("RUST_LOG").unwrap_or_else(|_| "info".into()),
        ))
        .with(tracing_subscriber::fmt::layer())
        .init();

    tracing::info!("world-engine starting");

    // TODO: gRPC server (tonic)
    // TODO: Tile grid 加载
    // TODO: 多玩家同步（Redis Pub/Sub）

    Ok(())
}
