//! REST API（Sprint 1 简化版）
//!
//! 与 gateway 通过 HTTP/JSON 通信：
//! - GET  /v1/tiles              → 列出所有 Tile
//! - GET  /v1/tiles/:id          → 查询单个 Tile
//! - POST /v1/world/move         → 移动玩家，更新 Tile 归属
//! - GET  /healthz               → 健康检查
//!
//! 真实部署里会把 `WorldGrid` 换成 sqlx 加载的 `tile` 表 + Redis Pub/Sub 广播。

use std::net::SocketAddr;
use std::sync::Arc;

use axum::{
    extract::{Path, State},
    http::StatusCode,
    response::IntoResponse,
    routing::{get, post},
    Json, Router,
};
use serde::{Deserialize, Serialize};
use tracing::{info, warn};

use crate::tile::Tile;
use crate::world_grid::WorldGrid;

#[derive(Clone)]
pub struct AppState {
    pub grid: Arc<WorldGrid>,
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
}

// ─── Handlers ──────────────────────────────────────────────────────────────

async fn healthz() -> impl IntoResponse {
    (StatusCode::OK, "ok")
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

async fn move_player(
    State(state): State<AppState>,
    Json(req): Json<MoveRequest>,
) -> Result<Json<MoveResponse>, (StatusCode, Json<ErrorBody>)> {
    // 简单校验：目标 Tile 必须存在
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

    state.grid.player_leave(&req.from_tile_id, &req.player_id);
    state.grid.player_enter(&req.to_tile_id, &req.player_id);

    info!(
        player_id = %req.player_id,
        from = %req.from_tile_id,
        to = %req.to_tile_id,
        x = req.x,
        y = req.y,
        "player moved"
    );

    Ok(Json(MoveResponse {
        player_id: req.player_id,
        current_tile_id: req.to_tile_id,
        x: req.x,
        y: req.y,
    }))
}

// ─── Router bootstrap ──────────────────────────────────────────────────────

pub fn router(state: AppState) -> Router {
    Router::new()
        .route("/healthz", get(healthz))
        .route("/v1/tiles", get(list_tiles))
        .route("/v1/tiles/:id", get(get_tile))
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

#[cfg(test)]
mod tests {
    use super::*;

    fn test_state() -> AppState {
        AppState {
            grid: Arc::new(WorldGrid::new()),
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
}