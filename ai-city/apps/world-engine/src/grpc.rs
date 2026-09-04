//! Sprint 3: tonic gRPC service 实现
//!
//! 4 个 RPC（来自 packages/proto/world.proto）：
//! - `Move(MoveRequest) → MoveResponse`：移动 + 校正
//! - `GetTile(GetTileRequest) → Tile`：单个 Tile 查询
//! - `SubscribePosition(SubscribeRequest) → stream PositionEvent`：按 tile_id 过滤的实时位置流
//! - `ComputePath(PathRequest) → PathResponse`：最短路径（Sprint 3 stub：返回 [start, end]）

use std::pin::Pin;
use std::sync::Arc;

use tokio::sync::mpsc;
use tokio_stream::wrappers::ReceiverStream;
use tonic::{Request, Response, Status};
use tracing::info;

use crate::redis_pub::RedisPub;
use crate::redis_sub::RedisSub;
use crate::world_grid::{PlayerPosition, WorldGrid};

// tonic-build 编译 world.proto 后生成的包路径
pub mod world_proto {
    tonic::include_proto!("aicity.world.v1");
}

pub use world_proto::{
    world_engine_server::{WorldEngine, WorldEngineServer},
    Building as ProtoBuilding,
    BuildingKind as ProtoBuildingKind,
    LodLevel as ProtoLodLevel,
    PathRequest, PathResponse,
    SubscribeRequest,
    Tile as ProtoTile,
    Vec2 as ProtoVec2,
    // messages
    GetTileRequest,
    MoveRequest, MoveResponse,
    PositionEvent,
};

// ─── Service ──────────────────────────────────────────────────────────────

#[derive(Clone)]
pub struct WorldEngineService {
    pub grid: Arc<WorldGrid>,
    pub redis_pub: Option<Arc<RedisPub>>,
    pub redis_sub: Option<Arc<RedisSub>>,
    pub channel_moved: String,
}

impl WorldEngineService {
    pub fn into_server(self) -> WorldEngineServer<Self> {
        WorldEngineServer::new(self)
    }
}

type PositionStream = Pin<Box<dyn tokio_stream::Stream<Item = Result<PositionEvent, Status>> + Send>>;

#[tonic::async_trait]
impl WorldEngine for WorldEngineService {
    type SubscribePositionStream = PositionStream;

    async fn r#move(
        &self,
        request: Request<MoveRequest>,
    ) -> Result<Response<MoveResponse>, Status> {
        let req = request.into_inner();
        if req.entity_id.is_empty() {
            return Err(Status::invalid_argument("entity_id is empty"));
        }

        // 目标 Tile 由 (x, y) 推算（与 REST /v1/world/move 保持一致）
        let target = req
            .target
            .ok_or_else(|| Status::invalid_argument("target is required"))?;
        let tile_id = crate::tile::Tile::from_xy(target.x, target.y, 100.0);
        let ts_ms = if req.ts_ms > 0 {
            req.ts_ms
        } else {
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .map(|d| d.as_millis() as i64)
                .unwrap_or(0)
        };
        let pos = PlayerPosition {
            player_id: req.entity_id.clone(),
            tile_id: tile_id.clone(),
            x: target.x,
            y: target.y,
            ts_ms,
        };
        self.grid.upsert_position(pos.clone());

        info!(
            entity_id = %req.entity_id,
            tile_id = %tile_id,
            predicted = req.predicted,
            "gRPC move"
        );

        // 异步广播（与 REST 路径一致）
        if let (Some(redis_pub), Some(payload)) =
            (&self.redis_pub, serde_json::to_string(&pos).ok())
        {
            let channel = self.channel_moved.clone();
            let pub_client = redis_pub.clone();
            tokio::spawn(async move {
                pub_client.publish(&channel, &payload).await;
            });
        }

        Ok(Response::new(MoveResponse {
            accepted: true,
            corrected_position: Some(ProtoVec2 {
                x: target.x,
                y: target.y,
            }),
            sequence: req.sequence,
            server_ts_ms: ts_ms,
        }))
    }

    async fn get_tile(
        &self,
        request: Request<GetTileRequest>,
    ) -> Result<Response<ProtoTile>, Status> {
        let req = request.into_inner();
        let tile = self
            .grid
            .get(&req.tile_id)
            .ok_or_else(|| Status::not_found(format!("tile {} not found", req.tile_id)))?;
        Ok(Response::new(tile_to_proto(&tile)))
    }

    async fn subscribe_position(
        &self,
        request: Request<SubscribeRequest>,
    ) -> Result<Response<Self::SubscribePositionStream>, Status> {
        let req = request.into_inner();
        let subscriber_id = if req.subscriber_id.is_empty() {
            "anonymous".to_string()
        } else {
            req.subscriber_id.clone()
        };
        let tile_id = req.tile_id;

        let redis_sub = self
            .redis_sub
            .clone()
            .ok_or_else(|| Status::unavailable("redis subscriber not configured"))?;

        let (tx, rx) = mpsc::channel::<Result<PositionEvent, Status>>(64);

        // payload 是 PlayerPosition JSON；只往匹配 tile_id 的订阅者发
        let mut stream = redis_sub.subscribe_with_filter(move |payload| {
            match serde_json::from_str::<PlayerPosition>(&payload) {
                Ok(pos) if tile_id.is_empty() || pos.tile_id == tile_id => Some(PositionEvent {
                    entity_id: pos.player_id,
                    tile_id: pos.tile_id,
                    position: Some(ProtoVec2 { x: pos.x, y: pos.y }),
                    ts_ms: pos.ts_ms,
                    predicted: false,
                    sequence: 0,
                }),
                _ => None,
            }
        });

        info!(subscriber_id, "gRPC subscribe_position started");
        tokio::spawn(async move {
            while let Some(event) = stream.recv().await {
                if tx.send(Ok(event)).await.is_err() {
                    break;
                }
            }
        });

        let stream: PositionStream = Box::pin(ReceiverStream::new(rx));
        Ok(Response::new(stream))
    }

    async fn compute_path(
        &self,
        request: Request<PathRequest>,
    ) -> Result<Response<PathResponse>, Status> {
        let req = request.into_inner();
        // Sprint 3 stub：直线路径（仅 start + end 两个 waypoint）
        // 真正的 A* / navmesh 留给 Sprint 4+
        let start = req.start.unwrap_or(ProtoVec2 { x: 0.0, y: 0.0 });
        let end = req.end.unwrap_or(ProtoVec2 { x: 0.0, y: 0.0 });
        let dx = end.x - start.x;
        let dy = end.y - start.y;
        let distance = (dx * dx + dy * dy).sqrt();

        Ok(Response::new(PathResponse {
            waypoints: vec![start, end],
            distance_m: distance,
        }))
    }
}

// ─── Adapter: tile::Tile → proto::Tile ─────────────────────────────────────

pub fn tile_to_proto(t: &crate::tile::Tile) -> ProtoTile {
    ProtoTile {
        id: t.id.clone(),
        center: Some(ProtoVec2 {
            x: t.center_x,
            y: t.center_y,
        }),
        size: t.size,
        lod_level: match t.lod_level {
            crate::tile::LodLevel::CBD => ProtoLodLevel::Cbd as i32,
            crate::tile::LodLevel::Residential => ProtoLodLevel::Residential as i32,
            crate::tile::LodLevel::Suburb => ProtoLodLevel::Suburb as i32,
        },
        npc_ids: t.npc_ids.clone(),
        player_ids: t.player_ids.clone(),
        buildings: t
            .buildings
            .iter()
            .map(|b| ProtoBuilding {
                id: b.id.clone(),
                kind: match b.kind {
                    crate::tile::BuildingKind::Tavern => ProtoBuildingKind::Tavern as i32,
                    crate::tile::BuildingKind::Shop => ProtoBuildingKind::Shop as i32,
                    crate::tile::BuildingKind::House => ProtoBuildingKind::House as i32,
                    crate::tile::BuildingKind::Park => ProtoBuildingKind::Park as i32,
                    crate::tile::BuildingKind::Road => ProtoBuildingKind::Road as i32,
                    crate::tile::BuildingKind::Plaza => ProtoBuildingKind::Plaza as i32,
                    crate::tile::BuildingKind::Office => ProtoBuildingKind::Office as i32,
                },
                polygon: b
                    .polygon
                    .iter()
                    .map(|(x, y)| ProtoVec2 { x: *x, y: *y })
                    .collect(),
            })
            .collect(),
    }
}

// ─── Tests ────────────────────────────────────────────────────────────────

#[cfg(test)]
mod tests {
    use super::*;
    use crate::tile::{Building, BuildingKind, LodLevel, Tile};

    fn sample_tile() -> Tile {
        Tile {
            id: "tile_0_0".into(),
            center_x: 50.0,
            center_y: 50.0,
            size: 100.0,
            buildings: vec![Building {
                id: "bldg_tavern_0_0".into(),
                kind: BuildingKind::Tavern,
                polygon: vec![(0.0, 0.0), (20.0, 0.0), (20.0, 15.0), (0.0, 15.0)],
            }],
            npc_ids: vec!["npc_wang_boss_001".into()],
            player_ids: vec![],
            lod_level: LodLevel::CBD,
        }
    }

    #[test]
    fn test_tile_to_proto_lod_mapping() {
        let p = tile_to_proto(&sample_tile());
        assert_eq!(p.id, "tile_0_0");
        assert_eq!(p.lod_level, ProtoLodLevel::Cbd as i32);
        assert_eq!(p.size, 100.0);
        assert_eq!(p.buildings.len(), 1);
        assert_eq!(p.buildings[0].kind, ProtoBuildingKind::Tavern as i32);
        assert_eq!(p.buildings[0].polygon.len(), 4);
        assert_eq!(p.npc_ids, vec!["npc_wang_boss_001".to_string()]);
    }

    #[test]
    fn test_tile_to_proto_residential() {
        let mut t = sample_tile();
        t.lod_level = LodLevel::Residential;
        let p = tile_to_proto(&t);
        assert_eq!(p.lod_level, ProtoLodLevel::Residential as i32);
    }

    #[test]
    fn test_tile_to_proto_suburb() {
        let mut t = sample_tile();
        t.lod_level = LodLevel::Suburb;
        let p = tile_to_proto(&t);
        assert_eq!(p.lod_level, ProtoLodLevel::Suburb as i32);
    }

    #[tokio::test]
    async fn test_compute_path_stub_returns_two_waypoints() {
        // 通过构造 service 直接调用 compute_path 校验逻辑
        let svc = WorldEngineService {
            grid: Arc::new(WorldGrid::new()),
            redis_pub: None,
            redis_sub: None,
            channel_moved: "test".into(),
        };
        let req = PathRequest {
            entity_id: "p1".into(),
            start: Some(ProtoVec2 { x: 0.0, y: 0.0 }),
            end: Some(ProtoVec2 { x: 30.0, y: 40.0 }),
        };
        let resp = svc.compute_path(Request::new(req)).await.unwrap();
        let inner = resp.into_inner();
        assert_eq!(inner.waypoints.len(), 2);
        assert!((inner.distance_m - 50.0).abs() < 0.001);
    }
}