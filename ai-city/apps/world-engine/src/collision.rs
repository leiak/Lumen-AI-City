//! 碰撞检测
//! 使用 AABB（轴对齐包围盒）作为基础检测，性能 < 0.1ms / 次

use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Copy, Serialize, Deserialize)]
pub struct AABB {
    pub min_x: f32,
    pub min_y: f32,
    pub max_x: f32,
    pub max_y: f32,
}

impl AABB {
    pub fn new(min_x: f32, min_y: f32, max_x: f32, max_y: f32) -> Self {
        Self { min_x, min_y, max_x, max_y }
    }

    /// AABB 与 AABB 相交检测
    pub fn intersects(&self, other: &AABB) -> bool {
        self.min_x <= other.max_x
            && self.max_x >= other.min_x
            && self.min_y <= other.max_y
            && self.max_y >= other.min_y
    }

    pub fn contains(&self, x: f32, y: f32) -> bool {
        x >= self.min_x && x <= self.max_x && y >= self.min_y && y <= self.max_y
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_intersects() {
        let a = AABB::new(0.0, 0.0, 10.0, 10.0);
        let b = AABB::new(5.0, 5.0, 15.0, 15.0);
        assert!(a.intersects(&b));

        let c = AABB::new(20.0, 20.0, 30.0, 30.0);
        assert!(!a.intersects(&c));
    }
}
