"""感知模块：从 Tick / Event 收集环境状态。"""
from __future__ import annotations

from agent_os.loop import Percept


class PerceptionPipeline:
    """感知管道：视野内 NPC / 玩家 / 物品 / 剧本事件。"""

    def __init__(self, vision_range_m: float = 50.0) -> None:
        self.vision_range_m = vision_range_m

    async def collect(self, agent_id: str, world_state: dict) -> list[Percept]:
        """从 world_state 中筛选 agent 视野内的实体。"""
        percepts: list[Percept] = []
        # TODO: 真实查询 - 暂用占位
        for entity in world_state.get("nearby_entities", []):
            percepts.append(
                Percept(
                    source="tick",
                    kind=entity.get("kind", "unknown"),
                    payload=entity,
                )
            )
        return percepts
