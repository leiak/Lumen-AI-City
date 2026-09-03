//! Neo4j Sync：从 Kafka 消费 PG 变更事件，异步投影到 Neo4j
//!
//! 详细设计见 docs/08-架构优化v1.md §26（写读分离）。

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    tracing_subscriber::fmt::init();
    tracing::info!("neo4j-sync starting");
    // TODO: 消费 Kafka CDC 事件 + 投影到 Neo4j
    Ok(())
}
