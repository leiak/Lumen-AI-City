//! 极简 PostgreSQL Simple Query 客户端（tokio TCP + 二进制 v3 协议）
//!
//! 避免引入 sqlx（其依赖链 parking_lot / ring / rustls 在 Windows 1.82 + GNU 工具链
//! 下缺 gcc.exe / dlltool.exe 编译不过；与 Sprint 1.5 时 redis-rs 触发栈溢出同类问题）。
//!
//! 只支持：
//! - Startup (trust 模式，无密码)
//! - Simple Query (Q 消息，返回字符串文本行）
//!
//! 不支持：MD5/SCRAM 鉴权、prepared statement (P)、COPY、事务、binary format。
//! 后续如需扩，先开 ADR。

use std::collections::HashMap;
use std::time::Duration;

use anyhow::{anyhow, bail, Context as _, Result};
use tokio::io::{AsyncReadExt, AsyncWriteExt, BufReader, BufWriter};
use tokio::net::TcpStream;
use tokio::sync::Mutex;

const PROTOCOL_VERSION_3_0: i32 = 196608;
/// trust 鉴权 OK（其余 code 都未实现）
const AUTH_OK: i32 = 0;

/// 一条 PG 消息（不含 tag 字节）
struct PgMsg {
    tag: u8,
    body: Vec<u8>,
}

pub struct PgConn {
    reader: Mutex<BufReader<tokio::net::tcp::OwnedReadHalf>>,
    writer: Mutex<BufWriter<tokio::net::tcp::OwnedWriteHalf>>,
}

/// `redis://user:pass@host:port/db` 风格的极简解析
/// 输入示例：
///   `postgres://aicity:aicity_dev@127.0.0.1:5432/aicity`
///   `postgresql://localhost/aicity`（缺 user 报错）
pub fn parse_pg_url(raw: &str) -> Result<PgConnectParams> {
    let s = raw.trim();
    let after = s
        .strip_prefix("postgres://")
        .or_else(|| s.strip_prefix("postgresql://"))
        .ok_or_else(|| anyhow!("not a postgres url: {}", raw))?;
    // user[:password]@host[:port]/dbname
    let (auth_host, db) = match after.split_once('/') {
        Some((a, d)) => (a, d),
        None => bail!("pg url missing /dbname: {}", raw),
    };
    let (auth, host_port) = match auth_host.rsplit_once('@') {
        Some((a, h)) => (a, h),
        None => bail!("pg url missing user@: {}", raw),
    };
    let (user, password) = match auth.split_once(':') {
        Some((u, p)) => (u, Some(p)),
        None => (auth, None),
    };
    if user.is_empty() {
        bail!("pg url missing user");
    }
    Ok(PgConnectParams {
        user: user.to_string(),
        password: password.unwrap_or("").to_string(),
        host_port: host_port.to_string(),
        database: db.to_string(),
    })
}

pub struct PgConnectParams {
    pub user: String,
    pub password: String,
    pub host_port: String,
    pub database: String,
}

/// 建连 + Startup + 等到第一个 ReadyForQuery（'Z'）
pub async fn connect(params: &PgConnectParams) -> Result<PgConn> {
    let stream = tokio::time::timeout(Duration::from_secs(2), TcpStream::connect(&params.host_port))
        .await
        .map_err(|_| anyhow!("pg connect timeout: {}", params.host_port))?
        .with_context(|| format!("pg connect failed: {}", params.host_port))?;
    stream.set_nodelay(true)?;
    let (read_half, write_half) = stream.into_split();
    let mut reader = BufReader::new(read_half);
    let mut writer = BufWriter::new(write_half);

    // ── 1. Startup message ──────────────────────────────────────────────────
    // length(4) + protocol(4) + [k\0v\0]* + \0
    let mut body: Vec<u8> = Vec::with_capacity(128);
    body.extend_from_slice(&PROTOCOL_VERSION_3_0.to_be_bytes());
    write_kv(&mut body, "user", &params.user);
    write_kv(&mut body, "database", &params.database);
    if !params.password.is_empty() {
        write_kv(&mut body, "password", &params.password);
    }
    body.push(0); // 终止
    let len = (body.len() + 4) as i32;
    writer.write_all(&len.to_be_bytes()).await?;
    writer.write_all(&body).await?;
    writer.flush().await?;

    // ── 2. 等到第一个 ReadyForQuery ('Z') ──────────────────────────────────
    loop {
        let msg = read_msg(&mut reader).await?;
        match msg.tag {
            b'R' => {
                let code = i32::from_be_bytes(msg.body[..4].try_into()?);
                if code != AUTH_OK {
                    bail!("pg auth required: code={} (only trust supported)", code);
                }
            }
            b'Z' => break,
            b'E' => bail!("pg startup error: {}", String::from_utf8_lossy(&msg.body)),
            _ => {} // 跳过 NoticeResponse / ParameterStatus
        }
    }

    Ok(PgConn {
        reader: Mutex::new(reader),
        writer: Mutex::new(writer),
    })
}

/// 简单查询（Q 消息）。返回每行 cell 文本值。
/// 注意：length() == -1 表示 SQL NULL。
pub async fn query_simple(conn: &PgConn, sql: &str) -> Result<Vec<Vec<Option<String>>>> {
    // Q message: tag(1) + length(4) + sql + \0
    let body_len = (sql.len() + 1 + 4) as i32;
    let mut buf = Vec::with_capacity(5 + sql.len() + 1);
    buf.push(b'Q');
    buf.extend_from_slice(&body_len.to_be_bytes());
    buf.extend_from_slice(sql.as_bytes());
    buf.push(0);

    let mut wguard = conn.writer.lock().await;
    wguard.write_all(&buf).await?;
    wguard.flush().await?;
    drop(wguard);

    let mut rows: Vec<Vec<Option<String>>> = Vec::new();
    let mut col_count: usize = 0;
    let mut rguard = conn.reader.lock().await;
    loop {
        // MutexGuard 不直接满足 AsyncReadExt bound；显式借出 &mut BufReader
        let msg = read_msg(&mut *rguard).await?;
        match msg.tag {
            b'T' => {
                // RowDescription: count(i16) + 每列(name\0 + table_oid(4) + col(2) + type_oid(4) + size(2) + typmod(4) + format(2))
                col_count = i16::from_be_bytes(msg.body[..2].try_into()?) as usize;
                // 不解析列定义（simple query 文本格式无需 type oid）
            }
            b'D' => {
                // DataRow: count(i16) + 每列(length(4) + data)
                let n = i16::from_be_bytes(msg.body[..2].try_into()?) as usize;
                if n != col_count {
                    bail!("DataRow column count mismatch: expected {col_count}, got {n}");
                }
                let mut off = 2usize;
                let mut row = Vec::with_capacity(n);
                for _ in 0..n {
                    let col_len = i32::from_be_bytes(msg.body[off..off + 4].try_into()?) as usize;
                    off += 4;
                    if col_len == 0xFFFF_FFFF {
                        row.push(None);
                    } else {
                        let val = std::str::from_utf8(&msg.body[off..off + col_len])
                            .map_err(|e| anyhow!("non-utf8 cell: {e}"))?
                            .to_string();
                        row.push(Some(val));
                        off += col_len;
                    }
                }
                rows.push(row);
            }
            b'C' => { /* CommandComplete（"SELECT 9" 之类）—— 忽略 */ }
            b'Z' => break,
            b'E' => bail!("pg query error: {}", String::from_utf8_lossy(&msg.body)),
            _ => {} // 跳过 NoticeResponse
        }
    }
    Ok(rows)
}

fn write_kv(buf: &mut Vec<u8>, k: &str, v: &str) {
    buf.extend_from_slice(k.as_bytes());
    buf.push(0);
    buf.extend_from_slice(v.as_bytes());
    buf.push(0);
}

async fn read_msg<R: AsyncReadExt + Unpin>(reader: &mut R) -> Result<PgMsg> {
    let mut header = [0u8; 5];
    reader.read_exact(&mut header).await?;
    let tag = header[0];
    let len = i32::from_be_bytes(header[1..5].try_into()?) as usize;
    if len < 4 {
        bail!("pg message length too small: {len}");
    }
    let mut body = vec![0u8; len - 4];
    reader.read_exact(&mut body).await?;
    Ok(PgMsg { tag, body })
}

/// 把 `[Vec<Option<String>>]` 映射成 HashMap<String, String>（按列名）
/// 仅用于 `SELECT a, b, c FROM ...` 这种已知列名场景
pub fn rows_to_maps(rows: Vec<Vec<Option<String>>>, col_names: &[&str]) -> Vec<HashMap<String, String>> {
    rows.into_iter()
        .map(|r| {
            col_names
                .iter()
                .zip(r)
                .filter_map(|(name, val)| val.map(|v| (name.to_string(), v)))
                .collect()
        })
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_pg_url_full() {
        let p = parse_pg_url("postgres://aicity:aicity_dev@127.0.0.1:5432/aicity").unwrap();
        assert_eq!(p.user, "aicity");
        assert_eq!(p.password, "aicity_dev");
        assert_eq!(p.host_port, "127.0.0.1:5432");
        assert_eq!(p.database, "aicity");
    }

    #[test]
    fn test_parse_pg_url_no_password() {
        let p = parse_pg_url("postgresql://postgres@localhost/mydb").unwrap();
        assert_eq!(p.user, "postgres");
        assert_eq!(p.password, "");
        assert_eq!(p.host_port, "localhost");
        assert_eq!(p.database, "mydb");
    }

    #[test]
    fn test_parse_pg_url_bad() {
        assert!(parse_pg_url("redis://localhost").is_err());
        assert!(parse_pg_url("postgres://localhost").is_err());
        assert!(parse_pg_url("postgres://@localhost/db").is_err());
    }

    /// 集成测试：需要本机 aicity-pg 在 127.0.0.1:5432 可达且 trust 鉴权
    #[tokio::test]
    #[ignore = "requires local aicity-pg on 127.0.0.1:5432 with trust auth"]
    async fn test_query_simple_live_pg() {
        let p = parse_pg_url("postgres://aicity@127.0.0.1:5432/aicity").unwrap();
        let conn = connect(&p).await.expect("connect");
        let rows = query_simple(
            &conn,
            "SELECT id, lod_level::text, npc_ids::text FROM tile ORDER BY id LIMIT 3",
        )
        .await
        .expect("query");
        assert_eq!(rows.len(), 3);
        assert_eq!(rows[0].len(), 3);
        // 第一列是 id，第二列是 lod_level，第三列是 npc_ids 数组文本
    }
}