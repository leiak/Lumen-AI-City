"""LOD 三档决策降级（§B.3 + §19.10）。

L0 背景板：行为树，无 LLM
L1 互动 NPC：行为树 + LLM 升级
L2 主角：全程 LLM
"""
from __future__ import annotations

from enum import IntEnum

from agent_os.config import settings


class LodLevel(IntEnum):
    L0 = 0  # 背景板
    L1 = 1  # 互动 NPC
    L2 = 2  # 主角


def compute_lod(
    player_distance_m: float,
    is_player_targeting: bool,
) -> LodLevel:
    """根据距离与玩家选中状态计算 LOD。"""
    if is_player_targeting:
        return LodLevel.L2
    if player_distance_m <= settings.lod0_distance_m:
        return LodLevel.L1
    return LodLevel.L0


def tick_interval_for_lod(lod: LodLevel) -> float:
    """LOD 对应的 tick 间隔。"""
    if lod == LodLevel.L2:
        return settings.cbd_tick_seconds  # 2s
    if lod == LodLevel.L1:
        return settings.residential_tick_seconds  # 5s
    return settings.suburb_tick_seconds  # 10s
