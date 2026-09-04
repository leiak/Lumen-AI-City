//! Sprint 2: Prometheus metrics (text format)
//!
//! 命名遵循 prometheus best practice：
//! - `_total` 后缀：counter 累加值
//! - 无后缀：gauge 当前值
//!
//! 数据源：直接读 `RedisPub` 内部的 `AtomicU64`（保证 JSON `/v1/_metrics` 与
//! Prometheus `/metrics` 数字一致）。

use crate::redis_pub::RedisPub;
use crate::world_grid::WorldGrid;

/// 把 RedisPub 状态 + grid 当前状态渲染为 Prometheus text format
///
/// 每次调用都重新构造 counter 数值并 emit（无状态、可重入）。
/// 不依赖 prometheus 库的 registry —— 简单可靠。
pub fn render(redis: Option<&RedisPub>, grid: &WorldGrid) -> String {
    let snap = redis.map(|p| p.stats());
    let (pub_n, conn_err, write_err, flush_err, ping_ok, ping_fail) = match snap {
        Some(s) => (
            s.messages_published as i64,
            s.connect_errors as i64,
            s.write_errors as i64,
            s.flush_errors as i64,
            s.ping_success as i64,
            s.ping_failure as i64,
        ),
        None => (0, 0, 0, 0, 0, 0),
    };
    let players = grid.player_count() as i64;
    let tiles = grid.list().len() as i64;

    let mut buf = String::with_capacity(2048);
    push_metric(
        &mut buf,
        "worldengine_redis_publish_total",
        "counter",
        "Number of successful Redis PUBLISH messages",
        pub_n,
    );
    push_metric(
        &mut buf,
        "worldengine_redis_connect_errors_total",
        "counter",
        "Number of Redis connect failures",
        conn_err,
    );
    push_metric(
        &mut buf,
        "worldengine_redis_write_errors_total",
        "counter",
        "Number of Redis write failures",
        write_err,
    );
    push_metric(
        &mut buf,
        "worldengine_redis_flush_errors_total",
        "counter",
        "Number of Redis flush failures",
        flush_err,
    );
    push_metric(
        &mut buf,
        "worldengine_redis_ping_success_total",
        "counter",
        "Number of successful Redis PING (got +PONG)",
        ping_ok,
    );
    push_metric(
        &mut buf,
        "worldengine_redis_ping_failure_total",
        "counter",
        "Number of failed Redis PING (timeout / non-+PONG)",
        ping_fail,
    );
    push_metric(
        &mut buf,
        "worldengine_tiles_loaded",
        "gauge",
        "Number of tiles currently loaded in WorldGrid",
        tiles,
    );
    push_metric(
        &mut buf,
        "worldengine_players_tracked",
        "gauge",
        "Number of players currently tracked in WorldGrid",
        players,
    );
    buf
}

fn push_metric(buf: &mut String, name: &str, kind: &str, help: &str, value: i64) {
    use std::fmt::Write as _;
    let _ = writeln!(buf, "# HELP {} {}", name, help);
    let _ = writeln!(buf, "# TYPE {} {}", name, kind);
    let _ = writeln!(buf, "{} {}", name, value);
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::redis_pub::RedisPub;

    #[test]
    fn test_render_no_redis() {
        let grid = WorldGrid::new();
        let body = render(None, &grid);
        assert!(body.contains("worldengine_redis_publish_total 0"));
        assert!(body.contains("worldengine_tiles_loaded 9"));
        assert!(body.contains("worldengine_players_tracked 0"));
        assert!(body.contains("# TYPE worldengine_redis_publish_total counter"));
        assert!(body.contains("# TYPE worldengine_tiles_loaded gauge"));
    }

    #[test]
    fn test_render_with_unreachable_redis() {
        // 不可达地址：stats 全 0 但不会失败
        let p = RedisPub::new("127.0.0.1:1");
        let grid = WorldGrid::new();
        let body = render(Some(&p), &grid);
        assert!(body.contains("worldengine_redis_publish_total 0"));
        assert!(body.contains("worldengine_tiles_loaded 9"));
    }
}