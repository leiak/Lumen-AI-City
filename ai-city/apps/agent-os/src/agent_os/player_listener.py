"""玩家位置 listener：维护玩家位置 + welcome 去重（Sprint 13 min slice）。

订阅 `aicity:player:moved`（world-engine 播）→ parse → 更新位置；
首个进入某 NPC home tile 的玩家只 welcome 一次（Q3 决策：重启即忘）。
纯逻辑，不含 Redis I/O —— 订阅 I/O 在 redis_sub.py，触发编排在 welcome_engine.py。
"""
from __future__ import annotations

import json
from dataclasses import dataclass


@dataclass
class PlayerPosition:
    player_id: str
    tile_id: str
    x: float
    y: float
    ts_ms: int


def parse_player_moved(payload: str) -> PlayerPosition | None:
    """解析 world-engine 发到 `aicity:player:moved` 的消息体。

    字段需与 `world-engine/src/rest.rs` 里 PlayerPosition 的 serde 输出一致
    （api-gateway 的 PlayerMovedPayload 也是同一契约）。解析失败返回 None，不抛。
    """
    try:
        d = json.loads(payload)
        return PlayerPosition(
            player_id=str(d["player_id"]),
            tile_id=str(d["tile_id"]),
            x=float(d["x"]),
            y=float(d["y"]),
            ts_ms=int(d["ts_ms"]),
        )
    except (ValueError, TypeError, KeyError):
        return None


class PlayerListener:
    """内存版玩家位置 + welcome 去重表（2.0 会接 PG）。"""

    def __init__(self) -> None:
        self.players: dict[str, PlayerPosition] = {}   # player_id → 最新位置
        self.welcomed_players: set[str] = set()        # 已 welcome 的 player_id

    def update_position(self, pos: PlayerPosition) -> None:
        """外部（Redis 订阅）更新玩家位置。"""
        self.players[pos.player_id] = pos

    def players_in_tile(self, tile_id: str) -> list[PlayerPosition]:
        """返回指定 tile 内的玩家（基于 tile_id 精确匹配）。"""
        return [p for p in self.players.values() if p.tile_id == tile_id]

    def is_in_range(self, player: PlayerPosition, npc_x: float, npc_y: float) -> bool:
        """Sprint 11 Q5 决策：同 tile（|Δx| < 100 ∧ |Δy| < 100）。"""
        return abs(player.x - npc_x) < 100 and abs(player.y - npc_y) < 100

    def is_welcomed(self, player_id: str) -> bool:
        return player_id in self.welcomed_players

    def mark_welcomed(self, player_id: str) -> None:
        self.welcomed_players.add(player_id)

    def clear_welcomed(self) -> None:
        """重启时调用（Q3 决策：welcome 状态存内存，重启即忘）。"""
        self.welcomed_players.clear()