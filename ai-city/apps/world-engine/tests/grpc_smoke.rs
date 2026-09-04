//! Sprint 3 gRPC 冒烟：进程内起 tonic server，用生成的 client 打 Move / GetTile / ComputePath。
//!
//! 不依赖 Redis / PG —— WorldGrid::new() 的 default_world() 即可覆盖。

use std::sync::Arc;
use std::time::Duration;

use world_core::grpc::world_proto::world_engine_client::WorldEngineClient;
use world_core::grpc::{GetTileRequest, MoveRequest, PathRequest, ProtoVec2, WorldEngineService};
use world_core::WorldGrid;

async fn spawn_server() -> String {
    // 端口 0 → 让 OS 分配，避免与本机已跑的 50051 冲突
    let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
    let addr = listener.local_addr().unwrap();

    let svc = WorldEngineService {
        grid: Arc::new(WorldGrid::new()),
        redis_pub: None,
        redis_sub: None,
        channel_moved: "test:moved".into(),
    };

    tokio::spawn(async move {
        tonic::transport::Server::builder()
            .add_service(svc.into_server())
            .serve_with_incoming(tokio_stream::wrappers::TcpListenerStream::new(listener))
            .await
            .unwrap();
    });

    // 给 server 一点起飞时间
    tokio::time::sleep(Duration::from_millis(150)).await;
    format!("http://{}", addr)
}

#[tokio::test]
async fn test_grpc_move_then_get_tile() {
    let endpoint = spawn_server().await;
    let mut client = WorldEngineClient::connect(endpoint).await.expect("connect");

    // Move → 落在 tile_0_0（size=100，(50,50) 归属 0,0）
    let resp = client
        .r#move(MoveRequest {
            entity_id: "player_smoke".into(),
            target: Some(ProtoVec2 { x: 50.0, y: 50.0 }),
            sequence: 7,
            predicted: true,
            ts_ms: 0,
        })
        .await
        .expect("move")
        .into_inner();

    assert!(resp.accepted);
    assert_eq!(resp.sequence, 7);
    let corrected = resp.corrected_position.expect("corrected_position");
    assert_eq!(corrected.x, 50.0);
    assert_eq!(corrected.y, 50.0);
    assert!(resp.server_ts_ms > 0);

    // GetTile → 该玩家应已进入 tile_0_0
    let tile = client
        .get_tile(GetTileRequest {
            tile_id: "tile_0_0".into(),
        })
        .await
        .expect("get_tile")
        .into_inner();

    assert_eq!(tile.id, "tile_0_0");
    assert_eq!(tile.size, 100.0);
    assert!(tile.player_ids.contains(&"player_smoke".to_string()));
}

#[tokio::test]
async fn test_grpc_move_rejects_empty_entity_id() {
    let endpoint = spawn_server().await;
    let mut client = WorldEngineClient::connect(endpoint).await.expect("connect");

    let err = client
        .r#move(MoveRequest {
            entity_id: String::new(),
            target: Some(ProtoVec2 { x: 0.0, y: 0.0 }),
            sequence: 1,
            predicted: false,
            ts_ms: 0,
        })
        .await
        .expect_err("should reject");

    assert_eq!(err.code(), tonic::Code::InvalidArgument);
}

#[tokio::test]
async fn test_grpc_get_tile_not_found() {
    let endpoint = spawn_server().await;
    let mut client = WorldEngineClient::connect(endpoint).await.expect("connect");

    let err = client
        .get_tile(GetTileRequest {
            tile_id: "tile_999_999".into(),
        })
        .await
        .expect_err("should 404");

    assert_eq!(err.code(), tonic::Code::NotFound);
}

#[tokio::test]
async fn test_grpc_compute_path() {
    let endpoint = spawn_server().await;
    let mut client = WorldEngineClient::connect(endpoint).await.expect("connect");

    let resp = client
        .compute_path(PathRequest {
            entity_id: "player_smoke".into(),
            start: Some(ProtoVec2 { x: 0.0, y: 0.0 }),
            end: Some(ProtoVec2 { x: 30.0, y: 40.0 }),
        })
        .await
        .expect("compute_path")
        .into_inner();

    assert_eq!(resp.waypoints.len(), 2);
    assert!((resp.distance_m - 50.0).abs() < 0.001);
}

#[tokio::test]
async fn test_grpc_subscribe_position_unavailable_without_redis() {
    let endpoint = spawn_server().await;
    let mut client = WorldEngineClient::connect(endpoint).await.expect("connect");

    let err = client
        .subscribe_position(world_core::grpc::SubscribeRequest {
            subscriber_id: "s1".into(),
            tile_id: "tile_0_0".into(),
        })
        .await
        .expect_err("no redis_sub configured");

    assert_eq!(err.code(), tonic::Code::Unavailable);
}
