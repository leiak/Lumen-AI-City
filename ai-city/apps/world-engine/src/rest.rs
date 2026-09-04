//! REST API（Sprint 1.5）
//!
//! 与 gateway 通过 HTTP/JSON 通信：
//! - GET  /v1/tiles                       → 列出所有 Tile
//! - GET  /v1/tiles/:id                   → 查询单个 Tile
//! - POST /v1/world/move                  → 移动玩家，更新 Tile 归属 + Redis publish
//! - GET  /v1/players/:id/position        → 查询玩家当前位置
//! - GET  /v1/tiles/:id/players           → 查询 Tile 上的玩家位置列表
//! - GET  /healthz                        → 健康检查
//!
//! 真实部署里会把 `WorldGrid` 换成 sqlx 加载的 `tile` 表 + Redis Pub/Sub 广播。

use std::net::SocketAddr;
use std::sync::Arc;
use std::time::{SystemTime, UNIX_EPOCH};

use axum::{
    extract::{Path, State},
    http::StatusCode,
    response::IntoResponse,
    routing::{get, post},
    Json, Router,
};
use serde::{Deserialize, Serialize};
use tracing::{info, warn};

use crate::redis_pub::RedisPub;
use crate::tile::Tile;
use crate::world_grid::{PlayerPosition, WorldGrid};

#[derive(Clone)]
pub struct AppState {
    pub grid: Arc<WorldGrid>,
    pub redis: Option<Arc<RedisPub>>,
    pub channel_moved: String,
}

#[derive(Serialize)]
pub struct HealthBody {
    pub status: &'static str,
    pub redis: &'static str,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub redis_stats: Option<crate::redis_pub::RedisStats>,
    pub tiles: usize,
    pub players_tracked: usize,
}

// ─── Response / Request DTOs ────────────────────────────────────────────────

#[derive(Serialize)]
pub struct ErrorBody {
    pub error: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub detail: Option<String>,
}

#[derive(Deserialize)]
pub struct MoveRequest {
    pub player_id: String,
    pub from_tile_id: String,
    pub to_tile_id: String,
    pub x: f32,
    pub y: f32,
}

#[derive(Serialize)]
pub struct MoveResponse {
    pub player_id: String,
    pub current_tile_id: String,
    pub x: f32,
    pub y: f32,
    pub ts_ms: i64,
}

// ─── Handlers ──────────────────────────────────────────────────────────────

async fn healthz(State(state): State<AppState>) -> impl IntoResponse {
    // 三态：not_configured (无 REDIS_URL) / ok (PING 回 +PONG) / unreachable
    let (redis, stats) = match &state.redis {
        Some(p) => {
            let ok = p.ping().await;
            let label = if ok { "ok" } else { "unreachable" };
            (label, Some(p.stats()))
        }
        None => ("not_configured", None),
    };
    let status = if redis == "ok" || redis == "not_configured" {
        "ok"
    } else {
        "degraded"
    };
    (
        StatusCode::OK,
        Json(HealthBody {
            status,
            redis,
            redis_stats: stats,
            tiles: state.grid.list().len(),
            players_tracked: state.grid.player_count(),
        }),
    )
}

async fn metrics(State(state): State<AppState>) -> impl IntoResponse {
    // 轻量 JSON metrics：可被 Prometheus 用 json_exporter 抓取，或仅供运维调试
    Json(serde_json::json!({
        "redis": state.redis.as_ref().map(|p| p.stats()),
        "tiles": state.grid.list().len(),
        "players_tracked": state.grid.player_count(),
        "channel_moved": state.channel_moved,
    }))
}

async fn list_tiles(State(state): State<AppState>) -> impl IntoResponse {
    Json(state.grid.list())
}

async fn get_tile(
    State(state): State<AppState>,
    Path(id): Path<String>,
) -> Result<Json<Tile>, (StatusCode, Json<ErrorBody>)> {
    match state.grid.get(&id) {
        Some(t) => Ok(Json(t)),
        None => Err((
            StatusCode::NOT_FOUND,
            Json(ErrorBody {
                error: "tile_not_found".into(),
                detail: Some(id),
            }),
        )),
    }
}

async fn get_player_position(
    State(state): State<AppState>,
    Path(player_id): Path<String>,
) -> Result<Json<PlayerPosition>, (StatusCode, Json<ErrorBody>)> {
    match state.grid.get_position(&player_id) {
        Some(p) => Ok(Json(p)),
        None => Err((
            StatusCode::NOT_FOUND,
            Json(ErrorBody {
                error: "position_not_found".into(),
                detail: Some(player_id),
            }),
        )),
    }
}

async fn list_tile_players(
    State(state): State<AppState>,
    Path(tile_id): Path<String>,
) -> Result<Json<Vec<PlayerPosition>>, (StatusCode, Json<ErrorBody>)> {
    if state.grid.get(&tile_id).is_none() {
        return Err((
            StatusCode::NOT_FOUND,
            Json(ErrorBody {
                error: "tile_not_found".into(),
                detail: Some(tile_id),
            }),
        ));
    }
    Ok(Json(state.grid.positions_in_tile(&tile_id)))
}

async fn move_player(
    State(state): State<AppState>,
    Json(req): Json<MoveRequest>,
) -> Result<Json<MoveResponse>, (StatusCode, Json<ErrorBody>)> {
    // 目标 Tile 必须存在
    if state.grid.get(&req.to_tile_id).is_none() {
        warn!(
            player_id = %req.player_id,
            to = %req.to_tile_id,
            "move rejected: target tile not found"
        );
        return Err((
            StatusCode::BAD_REQUEST,
            Json(ErrorBody {
                error: "tile_not_found".into(),
                detail: Some(req.to_tile_id),
            }),
        ));
    }

    let ts_ms = now_ms();
    let pos = PlayerPosition {
        player_id: req.player_id.clone(),
        tile_id: req.to_tile_id.clone(),
        x: req.x,
        y: req.y,
        ts_ms,
    };
    state.grid.upsert_position(pos.clone());

    info!(
        player_id = %req.player_id,
        from = %req.from_tile_id,
        to = %req.to_tile_id,
        x = req.x,
        y = req.y,
        "player moved"
    );

    // 异步广播到 Redis（失败不影响 move 主流程）
    if let Some(redis_pub) = &state.redis {
        let channel = state.channel_moved.clone();
        match serde_json::to_string(&pos) {
            Ok(payload) => {
                let pub_client = redis_pub.clone();
                tokio::spawn(async move {
                    pub_client.publish(&channel, &payload).await;
                });
            }
            Err(e) => {
                warn!("serialize position failed: {}", e);
            }
        }
    }

    Ok(Json(MoveResponse {
        player_id: req.player_id,
        current_tile_id: req.to_tile_id,
        x: req.x,
        y: req.y,
        ts_ms,
    }))
}

// ─── Router bootstrap ──────────────────────────────────────────────────────

pub fn router(state: AppState) -> Router {
    Router::new()
        .route("/healthz", get(healthz))
        .route("/v1/_metrics", get(metrics))
        .route("/v1/tiles", get(list_tiles))
        .route("/v1/tiles/:id", get(get_tile))
        .route("/v1/tiles/:id/players", get(list_tile_players))
        .route("/v1/players/:id/position", get(get_player_position))
        .route("/v1/world/move", post(move_player))
        .with_state(state)
}

pub async fn serve(addr: SocketAddr, state: AppState) -> anyhow::Result<()> {
    let app = router(state);
    info!(%addr, "world-engine REST listening");
    let listener = tokio::net::TcpListener::bind(addr).await?;
    axum::serve(listener, app).await?;
    Ok(())
}

fn now_ms() -> i64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_millis() as i64)
        .unwrap_or(0)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn test_state() -> AppState {
        AppState {
            grid: Arc::new(WorldGrid::new()),
            redis: None,
            channel_moved: "test:player:moved".into(),
        }
    }

    #[tokio::test]
    async fn test_list_tiles() {
        let state = test_state();
        let tiles = state.grid.list();
        assert!(!tiles.is_empty());
    }

    #[tokio::test]
    async fn test_get_tile_found() {
        let state = test_state();
        let t = state.grid.get("tile_0_0");
        assert!(t.is_some());
    }

    #[tokio::test]
    async fn test_get_tile_not_found() {
        let state = test_state();
        let t = state.grid.get("tile_does_not_exist");
        assert!(t.is_none());
    }

    #[tokio::test]
    async fn test_position_not_found() {
        let state = test_state();
        let p = state.grid.get_position("ghost");
        assert!(p.is_none());
    }

    #[tokio::test]
    async fn test_upsert_then_get_position() {
        let state = test_state();
        state.grid.upsert_position(PlayerPosition {
            player_id: "p1".into(),
            tile_id: "tile_0_0".into(),
            x: 1.0,
            y: 2.0,
            ts_ms: 1000,
        });
        let p = state.grid.get_position("p1").unwrap();
        assert_eq!(p.tile_id, "tile_0_0");
        assert!((p.x - 1.0).abs() < 0.001);
    }
}