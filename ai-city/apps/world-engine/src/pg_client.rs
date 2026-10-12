//! PostgreSQL simple-query adapter backed by the standard tokio-postgres
//! protocol implementation. The narrow API keeps existing tile and heartbeat
//! callers unchanged while adding SCRAM password authentication for prod.

use std::time::Duration;

use anyhow::{Context as _, Result};
use tokio::sync::Mutex;
use tokio::net::TcpStream;
use tokio_postgres::Config;

pub struct PgConn {
    client: Mutex<tokio_postgres::Client>,
}

/// `postgres://user:password@host:port/database` URL.
pub struct PgConnectParams {
    pub user: String,
    pub password: String,
    pub host_port: String,
    pub database: String,
}

/// Keep the existing parse boundary for callers and tests. tokio-postgres
/// performs the authoritative protocol negotiation when connecting.
pub fn parse_pg_url(raw: &str) -> Result<PgConnectParams> {
    let raw = raw.trim();
    let after_scheme = raw
        .strip_prefix("postgres://")
        .or_else(|| raw.strip_prefix("postgresql://"))
        .ok_or_else(|| anyhow::anyhow!("not a postgres url: {raw}"))?;
    let (auth_host, database) = after_scheme
        .split_once('/')
        .ok_or_else(|| anyhow::anyhow!("pg url missing /dbname: {raw}"))?;
    let database = database.split(['?', '&']).next().unwrap_or(database);
    let (auth, host_port) = auth_host
        .rsplit_once('@')
        .ok_or_else(|| anyhow::anyhow!("pg url missing user@: {raw}"))?;
    let (user, password) = match auth.split_once(':') {
        Some((user, password)) => (user, password),
        None => (auth, ""),
    };
    if user.is_empty() {
        anyhow::bail!("pg url missing user");
    }

    Ok(PgConnectParams {
        user: user.to_string(),
        password: password.to_string(),
        host_port: host_port.to_string(),
        database: database.to_string(),
    })
}

/// Connect with standard PostgreSQL auth negotiation. Internal production
// traffic stays unencrypted on the compose network; TLS and cluster networking
// are handled by the deployment layer.
pub async fn connect(params: &PgConnectParams) -> Result<PgConn> {
    let (host, port) = params
        .host_port
        .split_once(':')
        .unwrap_or((&params.host_port, "5432"));
    let config = Config::new()
        .user(&params.user)
        .password(params.password.as_bytes())
        .host(host)
        .port(port.parse().context("pg port is not valid")?)
        .dbname(&params.database)
        .to_owned();
    let socket_addr = format!("{}:{}", host, port);
    let stream = tokio::time::timeout(Duration::from_secs(5), TcpStream::connect(&socket_addr))
        .await
        .map_err(|_| anyhow::anyhow!("pg connect timeout: {}", socket_addr))?
        .with_context(|| format!("pg tcp connect failed: {}", socket_addr))?;
    let (client, connection) = tokio::time::timeout(
        Duration::from_secs(5),
        config.connect_raw(stream, tokio_postgres::NoTls),
    )
    .await
    .map_err(|_| anyhow::anyhow!("pg connect timeout: {}", socket_addr))?
    .with_context(|| format!("pg connect failed: {}", socket_addr))?;

    tokio::spawn(async move {
        if let Err(error) = connection.await {
            tracing::error!(error = ?error, "postgres connection closed");
        }
    });

    Ok(PgConn {
        client: Mutex::new(client),
    })
}

pub async fn query_simple(conn: &PgConn, sql: &str) -> Result<Vec<Vec<Option<String>>>> {
    let client = conn.client.lock().await;
    let result = client.simple_query(sql).await?;
    Ok(result
        .into_iter()
        .filter_map(|message| match message {
            tokio_postgres::SimpleQueryMessage::Row(row) => Some(
                (0..row.columns().len())
                    .map(|index| row.get(index).map(str::to_string))
                    .collect(),
            ),
            _ => None,
        })
        .collect())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_pg_url_full() {
        let params = parse_pg_url("postgres://aicity:aicity_dev@127.0.0.1:5432/aicity").unwrap();
        assert_eq!(params.user, "aicity");
        assert_eq!(params.password, "aicity_dev");
        assert_eq!(params.host_port, "127.0.0.1:5432");
        assert_eq!(params.database, "aicity");
    }

    #[test]
    fn test_parse_pg_url_no_password() {
        let params = parse_pg_url("postgresql://postgres@localhost/mydb").unwrap();
        assert_eq!(params.user, "postgres");
        assert_eq!(params.password, "");
        assert_eq!(params.host_port, "localhost");
        assert_eq!(params.database, "mydb");
    }

    #[test]
    fn test_parse_pg_url_bad() {
        assert!(parse_pg_url("redis://localhost").is_err());
        assert!(parse_pg_url("postgres://localhost").is_err());
        assert!(parse_pg_url("postgres://@localhost/db").is_err());
    }
}
