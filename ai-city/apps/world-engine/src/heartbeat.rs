//! ai-city/apps/world-engine/src/heartbeat.rs
//!
//! Sprint 12 (2.0 stage 1) — Task 48: NPC 心跳上报到 PG `a2a_npc_heartbeats` 表。
//!
//! 每 30s 调一次 upsert：记录 `last_seen_ms` + 简易系统指标（cpu_percent / memory_mb）。
//!
//! 实现注：本文件用 `crate::pg_client`（手写极简 PG Simple Query 客户端），**没用 sqlx**。
//! 原因见 `Cargo.toml` + `pg_client.rs` 顶部注释 —— sqlx 拉入 rustls / ring / parking_lot
//! 等 native 链，Windows 1.82 + GNU 工具链缺 gcc.exe / dlltool.exe 编译不过。
//!
//! 此外：
//! - `chrono` → `std::time::SystemTime`（项目其他模块一致，见 `grpc.rs::r#move`）
//! - `sys_info` → 占位 0.0/0（避免新增 native 依赖；后续可接 /proc/self/statm）
//! - `city` 列未加 —— `pg-schema-2.0.sql::a2a_npc_heartbeats` schema 只有
//!   `npc_id PK, last_seen_ms BIGINT NOT NULL, cpu_percent REAL, memory_mb INTEGER`。
//!   跨城信息由 `a2a_npc_routes.city` 持有，JOIN 即可。

use std::time::Duration;

use anyhow::{Context as _, Result};
use tracing::{debug, info, warn};

use crate::pg_client::{self, PgConn};

/// 心跳间隔。a2a 联邦层判定 alive 的阈值是 `now - last_seen_ms < 90s`（3 倍周期），
/// 见 docs/2.0/spec §6.4。
pub const HEARTBEAT_INTERVAL: Duration = Duration::from_secs(30);

/// 启动心跳上报 loop：每 30s 写一次 `a2a_npc_heartbeats`（upsert）。
///
/// 参数：
/// - `conn`：已 connect 的 PG 连接（trust 模式；compose `POSTGRES_HOST_AUTH_METHOD=trust` 保证）
/// - `npc_id`：当前 world-engine 实例的 NPC 标识（如 `npc_wang_boss_001`）
///
/// 行为：
/// - `INSERT ... ON CONFLICT (npc_id) DO UPDATE`：第一次写是 INSERT，之后每 30s upsert。
/// - 任一 tick 失败只 `warn!`，不 panic / 不退出循环（PG 短暂不可用不应让 world-engine 自杀）。
pub async fn start_heartbeat(conn: PgConn, npc_id: String) {
    info!(npc_id = %npc_id, interval_s = HEARTBEAT_INTERVAL.as_secs(), "heartbeat loop starting");
    let mut ticker = tokio::time::interval(HEARTBEAT_INTERVAL);
    ticker.set_missed_tick_behavior(tokio::time::MissedTickBehavior::Delay);

    loop {
        ticker.tick().await;
        let now_ms = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .map(|d| d.as_millis() as i64)
            .unwrap_or(0);
        // 占位：避免引入 sys_info / heim 等 native crate。后续接入 psutil 风格采样
        // （读 /proc/self/statm 或 getrusage(RUSAGE_SELF)）时替换此处。
        let cpu_percent: f32 = 0.0;
        let memory_mb: i32 = 0;

        let sql = format!(
            "INSERT INTO a2a_npc_heartbeats (npc_id, last_seen_ms, cpu_percent, memory_mb) \
             VALUES ('{}', {}, {}, {}) \
             ON CONFLICT (npc_id) DO UPDATE SET \
               last_seen_ms = EXCLUDED.last_seen_ms, \
               cpu_percent = EXCLUDED.cpu_percent, \
               memory_mb = EXCLUDED.memory_mb",
            escape_sql(&npc_id),
            now_ms,
            cpu_percent,
            memory_mb,
        );

        match pg_client::query_simple(&conn, &sql).await {
            Ok(_) => {
                debug!(npc_id = %npc_id, last_seen_ms = now_ms, "heartbeat upsert ok");
            }
            Err(e) => {
                warn!(npc_id = %npc_id, error = ?e, "heartbeat upsert failed (will retry next tick)");
            }
        }
    }
}

/// 便捷入口：从 `DATABASE_URL` 解析 + connect + 后台 spawn 心跳 task。
///
/// `main.rs` 在启动时调一次；失败返回 Err 让 main 走正常的 fail-fast 路径。
pub async fn bootstrap_from_env(npc_id: String) -> Result<()> {
    let url = std::env::var("DATABASE_URL").context("DATABASE_URL not set")?;
    let params = pg_client::parse_pg_url(&url).context("parse DATABASE_URL")?;
    let conn = pg_client::connect(&params)
        .await
        .with_context(|| format!("pg connect {}", params.host_port))?;
    tokio::spawn(start_heartbeat(conn, npc_id));
    Ok(())
}

/// 简易 SQL 单引号 escape：PG Simple Query 不支持参数化（`pg_client` 仅 Simple Query），
/// `npc_id` 来自 env 注入但仍做一次 escape 防止上游误传带引号的值。
fn escape_sql(s: &str) -> String {
    s.replace('\'', "''")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_escape_sql_passes_through_plain() {
        assert_eq!(escape_sql("npc_wang_boss_001"), "npc_wang_boss_001");
        assert_eq!(escape_sql(""), "");
    }

    #[test]
    fn test_escape_sql_doubles_single_quote() {
        assert_eq!(escape_sql("o'reilly"), "o''reilly");
        assert_eq!(escape_sql("a'b'c"), "a''b''c");
        assert_eq!(escape_sql("'"), "''");
    }

    #[test]
    fn test_interval_is_30s() {
        assert_eq!(HEARTBEAT_INTERVAL, Duration::from_secs(30));
    }
}
