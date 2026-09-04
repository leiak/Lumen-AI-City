//! Sprint 2+3: Prometheus metrics (text format)
//!
//! 命名遵循 prometheus best practice：
//! - `_total` 后缀：counter 累加值
//! - 无后缀：gauge 当前值
//!
//! 数据源：直接读 `RedisPub` / `RedisSub` 内部的 `AtomicU64`
//! （保证 JSON `/v1/_metrics` 与 Prometheus `/metrics` 数字一致）。

use crate::redis_pub::RedisPub;
use crate::redis_sub::RedisSub;
use crate::world_grid::WorldGrid;

/// 把 RedisPub + RedisSub 状态 + grid 当前状态渲染为 Prometheus text format
///
/// 每次调用都重新构造 counter 数值并 emit（无状态、可重入）。
/// 不依赖 prometheus 库的 registry —— 简单可靠。
pub fn render(redis: Option<&RedisPub>, redis_sub: Option<&RedisSub>, grid: &WorldGrid) -> String {
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
    let sub_snap = redis_sub.map(|s| s.stats());
    let (sub_recv, sub_parse_err, sub_conn_err, sub_reconnect) = match sub_snap {
        Some(s) => (
            s.messages_received as i64,
            s.parse_errors as i64,
            s.connect_errors as i64,
            s.reconnect_count as i64,
        ),
        None => (0, 0, 0, 0),
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
    // ── Sprint 3: RedisSub 订阅端 metrics ──────────────────────────────────
    push_metric(
        &mut buf,
        "worldengine_redis_sub_messages_total",
        "counter",
        "Number of Redis SUBSCRIBE messages received (payload parsed & forwarded)",
        sub_recv,
    );
    push_metric(
        &mut buf,
        "worldengine_redis_sub_parse_errors_total",
        "counter",
        "Number of Redis SUBSCRIBE payload parse failures",
        sub_parse_err,
    );
    push_metric(
        &mut buf,
        "worldengine_redis_sub_connect_errors_total",
        "counter",
        "Number of Redis SUBSCRIBE connect failures (post-start)",
        sub_conn_err,
    );
    push_metric(
        &mut buf,
        "worldengine_redis_sub_reconnects_total",
        "counter",
        "Number of Redis SUBSCRIBE reconnect attempts (backoff 1s->5s)",
        sub_reconnect,
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
        let body = render(None, None, &grid);
        assert!(body.contains("worldengine_redis_publish_total 0"));
        assert!(body.contains("worldengine_redis_sub_messages_total 0"));
        assert!(body.contains("worldengine_tiles_loaded 9"));
        assert!(body.contains("worldengine_players_tracked 0"));
        assert!(body.contains("# TYPE worldengine_redis_publish_total counter"));
        assert!(body.contains("# TYPE worldengine_redis_sub_messages_total counter"));
        assert!(body.contains("# TYPE worldengine_tiles_loaded gauge"));
    }

    #[test]
    fn test_render_with_unreachable_redis() {
        // 不可达地址：stats 全 0 但不会失败
        let p = RedisPub::new("127.0.0.1:1");
        let sub = RedisSub::new("redis://127.0.0.1:1", "test:ch");
        let grid = WorldGrid::new();
        let body = render(Some(&p), Some(&sub), &grid);
        assert!(body.contains("worldengine_redis_publish_total 0"));
        assert!(body.contains("worldengine_redis_sub_messages_total 0"));
        assert!(body.contains("worldengine_tiles_loaded 9"));
    }
}