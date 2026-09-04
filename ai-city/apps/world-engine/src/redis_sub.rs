//! 极简 Redis SUBSCRIBE 客户端（tokio TCP + RESP）
//!
//! 与 `redis_pub.rs` 同思路：避免 redis-rs 拉入大依赖链（Windows 1.82 + GNU 无 gcc/dlltool 编译不过）。
//! 只支持 SUBSCRIBE 到单一 channel；多个订阅者通过 tokio::sync::broadcast fan-out。
//!
//! 用法：
//!   let sub = RedisSub::new("redis://127.0.0.1:6379/0", "aicity:player:moved");
//!   sub.start().await?; // 启动后台 reader task；失败可重试
//!   let mut rx = sub.subscribe_with_filter(|payload| {
//!       // parse payload → Option<Event>
//!   });
//!   while let Some(event) = rx.recv().await { ... }

use std::collections::HashSet;
use std::sync::atomic::{AtomicU64, Ordering as AtomicOrdering};
use std::sync::Arc;
use std::time::Duration;

use anyhow::{anyhow, bail, Context as _, Result};
use serde::Serialize;
use tokio::io::{AsyncReadExt, AsyncWriteExt, BufReader};
use tokio::net::TcpStream;
use tokio::sync::{broadcast, mpsc, Mutex};
use tracing::{info, warn};

/// 订阅者内部计数器（与 redis_pub::RedisStats 镜像，便于 metrics 后续接入）
#[derive(Debug, Clone, Copy, Default, Serialize)]
pub struct RedisSubStats {
    pub messages_received: u64,
    pub parse_errors: u64,
    pub connect_errors: u64,
    pub reconnect_count: u64,
}

pub struct RedisSub {
    addr: String,
    channel: String,
    inner: Arc<Inner>,
}

struct Inner {
    /// 单 TCP 连接 → broadcast → 多 Receiver
    tx: broadcast::Sender<String>,
    /// 跟踪已启动的连接 task（避免重复 spawn）
    started: Mutex<bool>,
    stats: std::sync::Arc<SubStatsInner>,
}

struct SubStatsInner {
    messages_received: AtomicU64,
    parse_errors: AtomicU64,
    connect_errors: AtomicU64,
    reconnect_count: AtomicU64,
}

impl Default for SubStatsInner {
    fn default() -> Self {
        Self {
            messages_received: AtomicU64::new(0),
            parse_errors: AtomicU64::new(0),
            connect_errors: AtomicU64::new(0),
            reconnect_count: AtomicU64::new(0),
        }
    }
}

impl RedisSub {
    pub fn new(addr: impl Into<String>, channel: impl Into<String>) -> Self {
        let raw = addr.into();
        let parsed = parse_redis_addr(&raw).unwrap_or_else(|_| raw.clone());
        let (tx, _) = broadcast::channel(256);
        Self {
            addr: parsed,
            channel: channel.into(),
            inner: Arc::new(Inner {
                tx,
                started: Mutex::new(false),
                stats: std::sync::Arc::new(SubStatsInner::default()),
            }),
        }
    }

    pub fn addr(&self) -> &str {
        &self.addr
    }

    pub fn channel(&self) -> &str {
        &self.channel
    }

    pub fn stats(&self) -> RedisSubStats {
        RedisSubStats {
            messages_received: self.inner.stats.messages_received.load(AtomicOrdering::Relaxed),
            parse_errors: self.inner.stats.parse_errors.load(AtomicOrdering::Relaxed),
            connect_errors: self.inner.stats.connect_errors.load(AtomicOrdering::Relaxed),
            reconnect_count: self.inner.stats.reconnect_count.load(AtomicOrdering::Relaxed),
        }
    }

    /// 启动后台 reader task：建 TCP 连接、发 SUBSCRIBE、循环读 message → broadcast。
    /// 失败会重连（1s→5s 退避），通过 stats 暴露次数。
    pub async fn start(&self) -> Result<()> {
        let mut started = self.inner.started.lock().await;
        if *started {
            return Ok(());
        }
        *started = true;
        drop(started);

        let addr = self.addr.clone();
        let channel = self.channel.clone();
        let tx = self.inner.tx.clone();
        let stats = self.inner.stats.clone();

        tokio::spawn(async move {
            let mut backoff = Duration::from_secs(1);
            loop {
                match run_subscriber(&addr, &channel, &tx, &stats).await {
                    Ok(()) => {
                        // 正常退出（被关闭）→ 跳出循环
                        info!(addr = %addr, channel = %channel, "subscriber exited cleanly");
                        return;
                    }
                    Err(e) => {
                        stats.connect_errors.fetch_add(1, AtomicOrdering::Relaxed);
                        warn!(
                            addr = %addr,
                            channel = %channel,
                            error = %e,
                            backoff_ms = backoff.as_millis() as u64,
                            "subscriber error, retrying"
                        );
                        stats.reconnect_count.fetch_add(1, AtomicOrdering::Relaxed);
                        tokio::time::sleep(backoff).await;
                        backoff = (backoff * 2).min(Duration::from_secs(5));
                    }
                }
            }
        });

        Ok(())
    }

    /// 订阅过滤事件流。
    /// `filter` 接收原始 payload 字符串，返回 Some(T) 才转发。
    pub fn subscribe_with_filter<T, F>(&self, filter: F) -> RedisSubStream<T>
    where
        T: Send + 'static,
        F: Fn(String) -> Option<T> + Send + 'static,
    {
        let mut bc_rx = self.inner.tx.subscribe();
        let (tx, rx) = mpsc::channel::<T>(64);
        tokio::spawn(async move {
            loop {
                match bc_rx.recv().await {
                    Ok(payload) => {
                        if let Some(event) = filter(payload) {
                            if tx.send(event).await.is_err() {
                                break;
                            }
                        }
                    }
                    Err(broadcast::error::RecvError::Lagged(n)) => {
                        warn!(lagged = n, "subscriber lagged");
                    }
                    Err(broadcast::error::RecvError::Closed) => {
                        break;
                    }
                }
            }
        });
        RedisSubStream { rx }
    }
}

/// 后台 reader task：建 TCP、发 SUBSCRIBE、循环读 message → broadcast
async fn run_subscriber(
    addr: &str,
    channel: &str,
    tx: &broadcast::Sender<String>,
    stats: &std::sync::Arc<SubStatsInner>,
) -> Result<()> {
    let stream = tokio::time::timeout(Duration::from_secs(2), TcpStream::connect(addr))
        .await
        .map_err(|_| anyhow!("connect timeout: {}", addr))?
        .with_context(|| format!("connect failed: {}", addr))?;
    stream.set_nodelay(true)?;
    let (read_half, mut write_half) = stream.into_split();

    // SUBSCRIBE
    let mut buf = Vec::with_capacity(64 + channel.len());
    buf.extend_from_slice(b"*2\r\n");
    buf.extend_from_slice(b"$9\r\nSUBSCRIBE\r\n");
    buf.extend_from_slice(format!("${}\r\n{}\r\n", channel.len(), channel).as_bytes());
    write_half.write_all(&buf).await?;
    write_half.flush().await?;

    let mut reader = BufReader::new(read_half);
    let mut scratch = String::new();
    let mut subscribed = HashSet::new();

    loop {
        let line = read_line(&mut reader, &mut scratch).await?;
        if line.is_empty() {
            continue;
        }
        // 期望: *3\r\n$<n>\r\n<message|message|message>\r\n...
        if !line.starts_with('*') {
            // 跳过早到的非数组消息
            continue;
        }
        let n_elements: usize = line[1..].parse().unwrap_or(0);
        if n_elements != 3 {
            // 非订阅消息帧，跳过
            for _ in 0..n_elements {
                let _ = read_value(&mut reader, &mut scratch).await?;
            }
            continue;
        }
        let kind = read_bulk_string(&mut reader, &mut scratch).await?;
        let chan = read_bulk_string(&mut reader, &mut scratch).await?;
        // 第三个元素：message 时是 bulk string（payload）；subscribe 确认时是 integer(:N)
        let payload = read_value(&mut reader, &mut scratch).await?;
        if kind == "message" {
            stats.messages_received.fetch_add(1, AtomicOrdering::Relaxed);
            // fan-out（无订阅者时 silently drop）
            let _ = tx.send(payload);
        } else if kind == "subscribe" {
            subscribed.insert(chan.clone());
            info!(channel = %chan, "redis subscribed");
        }
    }
}

/// 读一行（不含 \r\n）
async fn read_line<R: AsyncReadExt + Unpin>(
    r: &mut R,
    scratch: &mut String,
) -> Result<String> {
    scratch.clear();
    let mut byte = [0u8; 1];
    loop {
        let n = r.read(&mut byte).await?;
        if n == 0 {
            bail!("eof");
        }
        if byte[0] == b'\n' {
            if scratch.ends_with('\r') {
                scratch.pop();
            }
            return Ok(scratch.clone());
        }
        scratch.push(byte[0] as char);
    }
}

/// 读一个 RESP 标量：bulk string（$）/ integer（:）/ simple string（+）/ error（-）
async fn read_value<R: AsyncReadExt + Unpin>(
    r: &mut R,
    scratch: &mut String,
) -> Result<String> {
    let header = read_line(r, scratch).await?;
    match header.chars().next() {
        Some('$') => {
            let len: i64 = header[1..].parse()?;
            if len < 0 {
                return Ok(String::new()); // NULL bulk
            }
            let mut buf = vec![0u8; len as usize];
            r.read_exact(&mut buf).await?;
            let mut tail = [0u8; 2]; // 尾部 \r\n
            r.read_exact(&mut tail).await?;
            Ok(String::from_utf8_lossy(&buf).into_owned())
        }
        Some(':') | Some('+') => Ok(header[1..].to_string()),
        Some('-') => bail!("redis error: {}", &header[1..]),
        _ => bail!("unexpected resp value: {}", header),
    }
}

/// 读一个 RESP 批量字符串（$<len>\r\n<data>\r\n）
async fn read_bulk_string<R: AsyncReadExt + Unpin>(
    r: &mut R,
    scratch: &mut String,
) -> Result<String> {
    let header = read_line(r, scratch).await?;
    if !header.starts_with('$') {
        bail!("expected bulk string, got: {}", header);
    }
    let len: i64 = header[1..].parse()?;
    if len < 0 {
        return Ok(String::new()); // NULL bulk
    }
    let mut buf = vec![0u8; len as usize];
    r.read_exact(&mut buf).await?;
    // 读尾部 \r\n
    let mut tail = [0u8; 2];
    r.read_exact(&mut tail).await?;
    Ok(String::from_utf8_lossy(&buf).into_owned())
}

/// 流封装（薄包装，便于未来加 close/err）
pub struct RedisSubStream<T> {
    rx: mpsc::Receiver<T>,
}

impl<T> RedisSubStream<T> {
    pub async fn recv(&mut self) -> Option<T> {
        self.rx.recv().await
    }
}

// ─── 复用 redis_pub.rs 的 URL 解析（保持一致） ─────────────────────────────

fn parse_redis_addr(raw: &str) -> Result<String> {
    let s = raw.trim();
    let after_scheme = s.strip_prefix("redis://").unwrap_or(s);
    let host_port = after_scheme.split('/').next().unwrap_or("");
    let host_port = host_port.rsplit('@').next().unwrap_or("");
    if host_port.is_empty() {
        bail!("invalid redis url: {}", raw);
    }
    Ok(host_port.to_string())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_redis_addr() {
        assert_eq!(parse_redis_addr("redis://localhost:6379/0").unwrap(), "localhost:6379");
        assert_eq!(parse_redis_addr("redis://:pw@127.0.0.1:6379/2").unwrap(), "127.0.0.1:6379");
        assert_eq!(parse_redis_addr("localhost:6379").unwrap(), "localhost:6379");
    }

    #[test]
    fn test_stats_initial_zero() {
        let s = RedisSub::new("redis://127.0.0.1:1", "test:ch");
        let st = s.stats();
        assert_eq!(st.messages_received, 0);
        assert_eq!(st.parse_errors, 0);
        assert_eq!(st.connect_errors, 0);
    }

    /// 集成测试：需要本机 127.0.0.1:6379 可达的 Redis。
    /// 流程：起一个 publisher（用 redis_pub）发一条 → subscribe_with_filter 收到 → 断言。
    /// `cargo test -- --ignored` 才跑。
    #[tokio::test]
    #[ignore = "requires local redis on 127.0.0.1:6379"]
    async fn test_subscribe_receives_published_message() {
        use crate::redis_pub::RedisPub;

        let sub = RedisSub::new("redis://127.0.0.1:6379/0", "test:sub:ch");
        sub.start().await.expect("start");

        // 给 reader task 一点时间建连接
        tokio::time::sleep(Duration::from_millis(300)).await;

        let mut rx = sub.subscribe_with_filter(|payload| {
            // 只放过包含 "marker" 的 payload
            if payload.contains("marker") {
                Some(payload)
            } else {
                None
            }
        });

        // 发两条：一条有 marker，一条无 marker
        let pub_client = RedisPub::new("redis://127.0.0.1:6379/0");
        pub_client.publish("test:sub:ch", "{\"marker\":42}").await;
        pub_client.publish("test:sub:ch", "no-marker").await;

        let got = tokio::time::timeout(Duration::from_secs(2), rx.recv())
            .await
            .expect("timeout")
            .expect("recv");
        assert!(got.contains("marker"));

        // 验证 stats
        let st = sub.stats();
        assert!(st.messages_received >= 2);
    }
}