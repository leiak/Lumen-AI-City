//! CDC Consumer：从 PostgreSQL WAL 同步变更到 Kafka
//!
//! 详细设计见 docs/08-架构优化v1.md §26（异步图投影）。

use std::env;

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    tracing_subscriber::fmt::init();

    let pg_url = env::var("DATABASE_URL")?;
    let kafka = env::var("KAFKA_BROKERS")?;

    tracing::info!("cdc-consumer starting pg={} kafka={}", pg_url, kafka);

    // TODO: 实现 logical replication slot + Kafka producer

    Ok(())
}
