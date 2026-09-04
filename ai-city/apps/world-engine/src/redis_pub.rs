//! 极简 Redis PUBLISH 客户端（tokio TCP + RESP）
//!
//! 避免引入 redis-rs（其依赖链含 winapi 等大 crate，本机编译会触发栈溢出）。
//! 只支持 `PUBLISH` 命令：api-gateway 端用 go-redis 订阅 + 写 PG。
//!
//! RESP 协议片段（PUBLISH channel payload）：
//!   *3\r\n$7\r\nPUBLISH\r\n$<len_ch>\r\n<channel>\r\n$<len_pl>\r\n<payload>\r\n

use std::sync::atomic::{AtomicU64, Ordering};
use std::time::Duration;
use tokio::io::{AsyncReadExt, AsyncWriteExt, BufWriter};
use tokio::net::TcpStream;
use tokio::sync::Mutex;
use tracing::warn;

use serde::Serialize;

/// RedisPub 内部计数器快照。lock-free 原子读，多 tokio 任务并发安全。
#[derive(Debug, Clone, Copy, Default, Serialize)]
pub struct RedisStats {
    pub messages_published: u64,
    pub connect_errors: u64,
    pub write_errors: u64,
    pub flush_errors: u64,
    pub ping_success: u64,
    pub ping_failure: u64,
}

#[derive(Clone)]
pub struct RedisPub {
    addr: String,
    conn: std::sync::Arc<Mutex<Option<BufWriter<TcpStream>>>>,
    stats: std::sync::Arc<RedisStatsInner>,
}

struct RedisStatsInner {
    messages_published: AtomicU64,
    connect_errors: AtomicU64,
    write_errors: AtomicU64,
    flush_errors: AtomicU64,
    ping_success: AtomicU64,
    ping_failure: AtomicU64,
}

impl Default for RedisStatsInner {
    fn default() -> Self {
        Self {
            messages_published: AtomicU64::new(0),
            connect_errors: AtomicU64::new(0),
            write_errors: AtomicU64::new(0),
            flush_errors: AtomicU64::new(0),
            ping_success: AtomicU64::new(0),
            ping_failure: AtomicU64::new(0),
        }
    }
}

impl RedisPub {
    pub fn new(addr: impl Into<String>) -> Self {
        let raw = addr.into();
        let parsed = parse_redis_addr(&raw).unwrap_or_else(|_| raw.clone());
        Self {
            addr: parsed,
            conn: std::sync::Arc::new(Mutex::new(None)),
            stats: std::sync::Arc::new(RedisStatsInner::default()),
        }
    }

    /// 读取当前计数器快照
    pub fn stats(&self) -> RedisStats {
        RedisStats {
            messages_published: self.stats.messages_published.load(Ordering::Relaxed),
            connect_errors: self.stats.connect_errors.load(Ordering::Relaxed),
            write_errors: self.stats.write_errors.load(Ordering::Relaxed),
            flush_errors: self.stats.flush_errors.load(Ordering::Relaxed),
            ping_success: self.stats.ping_success.load(Ordering::Relaxed),
            ping_failure: self.stats.ping_failure.load(Ordering::Relaxed),
        }
    }

    async fn ensure_conn(&self) -> anyhow::Result<()> {
        let mut guard = self.conn.lock().await;
        if guard.is_some() {
            return Ok(());
        }
        let stream = tokio::time::timeout(Duration::from_secs(2), TcpStream::connect(&self.addr))
            .await
            .map_err(|_| {
                self.stats.connect_errors.fetch_add(1, Ordering::Relaxed);
                anyhow::anyhow!("redis connect timeout")
            })?
            .map_err(|e| {
                self.stats.connect_errors.fetch_add(1, Ordering::Relaxed);
                e
            })?;
        stream.set_nodelay(true)?;
        *guard = Some(BufWriter::new(stream));
        Ok(())
    }

    /// 异步发布一条消息；连接失败/写入失败仅 warn，不影响主流程
    pub async fn publish(&self, channel: &str, payload: &str) {
        if let Err(e) = self.ensure_conn().await {
            warn!(addr = %self.addr, error = %e, "redis connect failed");
            return;
        }
        let mut guard = self.conn.lock().await;
        let Some(writer) = guard.as_mut() else {
            return;
        };

        let mut buf = Vec::with_capacity(channel.len() + payload.len() + 32);
        buf.extend_from_slice(b"*3\r\n");
        write_bulk(&mut buf, b"PUBLISH");
        write_bulk(&mut buf, channel.as_bytes());
        write_bulk(&mut buf, payload.as_bytes());

        if let Err(e) = writer.write_all(&buf).await {
            warn!(error = %e, "redis write failed, will reconnect on next call");
            self.stats.write_errors.fetch_add(1, Ordering::Relaxed);
            *guard = None;
            return;
        }
        if let Err(e) = writer.flush().await {
            warn!(error = %e, "redis flush failed");
            self.stats.flush_errors.fetch_add(1, Ordering::Relaxed);
            *guard = None;
            return;
        }
        // 不读响应（PUBLISH 的响应只有整数 +OK，单工；订阅端会收到消息）
        self.stats.messages_published.fetch_add(1, Ordering::Relaxed);
    }

    /// 健康探针：建立一次独立短连接，发 PING 读 +PONG；1s 超时。
    /// 与 publish() 的共享连接解耦，避免 ping 干扰 publish 缓冲。
    pub async fn ping(&self) -> bool {
        let stream = match tokio::time::timeout(Duration::from_secs(1), TcpStream::connect(&self.addr)).await {
            Ok(Ok(s)) => s,
            Ok(Err(e)) => {
                warn!(addr = %self.addr, error = %e, "redis ping: connect failed");
                self.stats.ping_failure.fetch_add(1, Ordering::Relaxed);
                return false;
            }
            Err(_) => {
                warn!(addr = %self.addr, "redis ping: connect timeout");
                self.stats.ping_failure.fetch_add(1, Ordering::Relaxed);
                return false;
            }
        };
        let mut stream = stream;
        if let Err(e) = stream.write_all(b"*1\r\n$4\r\nPING\r\n").await {
            warn!(error = %e, "redis ping: write failed");
            self.stats.ping_failure.fetch_add(1, Ordering::Relaxed);
            return false;
        }
        let mut buf = [0u8; 7];
        let ok = match stream.read_exact(&mut buf).await {
            Ok(_) => &buf == b"+PONG\r\n",
            Err(e) => {
                warn!(error = %e, "redis ping: read failed");
                false
            }
        };
        if ok {
            self.stats.ping_success.fetch_add(1, Ordering::Relaxed);
        } else {
            if !matches!(&buf, b"+PONG\r\n") {
                warn!(reply = ?buf, "redis ping: unexpected reply");
            }
            self.stats.ping_failure.fetch_add(1, Ordering::Relaxed);
        }
        ok
    }
}

fn write_bulk(buf: &mut Vec<u8>, s: &[u8]) {
    buf.push(b'$');
    buf.extend_from_slice(s.len().to_string().as_bytes());
    buf.extend_from_slice(b"\r\n");
    buf.extend_from_slice(s);
    buf.extend_from_slice(b"\r\n");
}

/// 极简 redis URL 解析：只提取 host:port，丢弃 scheme/path/password。
/// 输入示例：`redis://localhost:6379/0` → `localhost:6379`
///         `redis://:pw@127.0.0.1:6379/2` → `127.0.0.1:6379`
///         `localhost:6379`（无 scheme）→  原样返回
fn parse_redis_addr(raw: &str) -> anyhow::Result<String> {
    let s = raw.trim();
    let after_scheme = s.strip_prefix("redis://").unwrap_or(s);
    // 去掉 `/db` 路径
    let host_port = after_scheme.split('/').next().unwrap_or("");
    // 去掉 `:password@`（不支持 AUTH）
    let host_port = host_port.rsplit('@').next().unwrap_or("");
    if host_port.is_empty() {
        anyhow::bail!("invalid redis url: {}", raw);
    }
    Ok(host_port.to_string())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_write_bulk() {
        let mut buf = Vec::new();
        buf.extend_from_slice(b"*3\r\n");
        write_bulk(&mut buf, b"PUBLISH");
        write_bulk(&mut buf, b"chan");
        write_bulk(&mut buf, b"hello");
        let expected = b"*3\r\n$7\r\nPUBLISH\r\n$4\r\nchan\r\n$5\r\nhello\r\n";
        assert_eq!(buf, expected);
    }

    #[test]
    fn test_parse_redis_addr() {
        assert_eq!(parse_redis_addr("redis://localhost:6379/0").unwrap(), "localhost:6379");
        assert_eq!(parse_redis_addr("redis://localhost:6379").unwrap(), "localhost:6379");
        assert_eq!(parse_redis_addr("redis://:pw@127.0.0.1:6379/2").unwrap(), "127.0.0.1:6379");
        assert_eq!(parse_redis_addr("localhost:6379").unwrap(), "localhost:6379");
        assert_eq!(parse_redis_addr("redis://[::1]:6379/0").unwrap(), "[::1]:6379");
        assert!(parse_redis_addr("redis://").is_err());
    }

    /// 集成测试：需要本机 127.0.0.1:6379 可达的 Redis。
    /// `cargo test -- --ignored` 才跑。
    #[tokio::test]
    #[ignore = "requires local redis on 127.0.0.1:6379"]
    async fn test_ping_live_redis() {
        let p = RedisPub::new("redis://127.0.0.1:6379/0");
        assert!(p.ping().await);
    }

    #[tokio::test]
    async fn test_ping_unreachable() {
        // 不可达地址（假设 1 没人监听的高位端口）
        let p = RedisPub::new("127.0.0.1:1");
        assert!(!p.ping().await);
        let s = p.stats();
        assert_eq!(s.ping_failure, 1);
        assert_eq!(s.ping_success, 0);
    }

    #[tokio::test]
    async fn test_stats_initial_zero() {
        let p = RedisPub::new("redis://127.0.0.1:65530");
        let s = p.stats();
        assert_eq!(s.messages_published, 0);
        assert_eq!(s.connect_errors, 0);
        assert_eq!(s.write_errors, 0);
        assert_eq!(s.flush_errors, 0);
    }

    #[tokio::test]
    async fn test_publish_failure_increments_connect_errors() {
        let p = RedisPub::new("redis://127.0.0.1:65530");
        p.publish("test:ch", "{}").await;
        let s = p.stats();
        assert!(s.connect_errors >= 1, "expected connect_errors >= 1, got {}", s.connect_errors);
        assert_eq!(s.messages_published, 0);
    }
}