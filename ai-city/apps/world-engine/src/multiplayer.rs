//! 多玩家同步（Redis Pub/Sub + Kafka）
//! §E.6 客户端预测 + 服务端协调

use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PositionEvent {
    pub entity_id: String,
    pub tile_id: String,
    pub x: f32,
    pub y: f32,
    pub ts_ms: i64,
    pub predicted: bool,
    pub sequence: u64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TileSubscription {
    pub tile_id: String,
    pub subscriber_id: String,
}
