//! 移动逻辑
//! 玩家和 NPC 都基于此模块

use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Copy, Serialize, Deserialize)]
pub struct Vec2 {
    pub x: f32,
    pub y: f32,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Movement {
    pub entity_id: String,
    pub position: Vec2,
    pub velocity: Vec2,
    pub target: Option<Vec2>,
    pub speed: f32, // m/s
}

impl Movement {
    pub fn new(entity_id: String, position: Vec2, speed: f32) -> Self {
        Self {
            entity_id,
            position,
            velocity: Vec2 { x: 0.0, y: 0.0 },
            target: None,
            speed,
        }
    }

    /// 向目标移动一步（dt 秒）
    pub fn step(&mut self, dt: f32) {
        if let Some(target) = self.target {
            let dx = target.x - self.position.x;
            let dy = target.y - self.position.y;
            let dist = (dx * dx + dy * dy).sqrt();

            if dist < 0.1 {
                self.target = None;
                self.velocity = Vec2 { x: 0.0, y: 0.0 };
                return;
            }

            let nx = dx / dist;
            let ny = dy / dist;
            self.velocity = Vec2 { x: nx * self.speed, y: ny * self.speed };
            self.position.x += self.velocity.x * dt;
            self.position.y += self.velocity.y * dt;
        }
    }

    pub fn set_target(&mut self, target: Vec2) {
        self.target = Some(target);
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_movement_step() {
        let mut m = Movement::new("p1".into(), Vec2 { x: 0.0, y: 0.0 }, 1.0);
        m.set_target(Vec2 { x: 10.0, y: 0.0 });
        m.step(1.0);
        assert!((m.position.x - 1.0).abs() < 0.001);
    }
}
