# world-engine

> **职责**：城市物理 / Tile / 碰撞 / 移动 / 多玩家同步
>
> **语言**：Rust 1.82 + tonic (gRPC)
>
> **性能目标**：单实例 10K NPC + 1K 玩家，TPS > 1000
>
> **关键文档**：[docs/01-愿景.md §5](../../docs/01-愿景.md) / [docs/11-技术细节与玩法模式.md §B.1](../../docs/11-技术细节与玩法模式.md)

## 模块

- `tile.rs` - Tile 数据结构
- `movement.rs` - 移动逻辑
- `collision.rs` - AABB 碰撞
- `multiplayer.rs` - 多玩家同步（§E.6）

## 端口

`50051`（gRPC）
