"""每 move_tick 给 list_enabled() 里每个 NPC 选一个新位置并发布 aicity:npc_moved。

Sprint 13：agent-os 作为行为引擎决定 NPC 下一步去哪个 tile（纯脚本，不接 LLM），
把 {npc_id, tile_id, x, y, ts_ms} 发到 Redis npc_moved 频道 → ws-gateway 广播 → web 挪圆点。
MVP 不写 world-engine 位置表（权威坐标列为下一步硬化，见
docs/superpowers/specs/2026-09-19-sprint13-npc-movement-design.md）。
与 SayScheduler 不互斥：move 与 say 各自独立 tick，靠 move_tick > say_tick 错峰。
"""
from __future__ import annotations

import asyncio
import json
import logging
import random
import time
from collections.abc import Callable
from typing import Protocol

from agent_os.npc_registry import NpcRegistry

logger = logging.getLogger(__name__)

# tile 边长（与 world-engine 一致，仅用于默认 chooser 推导世界坐标）
TILE_SIZE = 100.0


class _PublisherLike(Protocol):
    def publish(self, channel: str, payload: str) -> None: ...


# 默认 chooser 的返回： (tile_id, x, y)
Chooser = Callable[[object, object | None, int, random.Random], tuple[str, float, float]]


def _tile_center(tile_id: str) -> tuple[float, float]:
    tx, ty = _parse_tile(tile_id)
    return tx * TILE_SIZE + TILE_SIZE / 2.0, ty * TILE_SIZE + TILE_SIZE / 2.0


def default_chooser(tpl, current, step: int, rng: random.Random) -> tuple[str, float, float]:
    """默认走法：优先按模板 walk.tiles 循环走（1.0 王老板「3 步走法」），
    否则退化为 home±1 随机邻格（MVP 不出 3×3 网格）。目标 tile 中心加抖动让"走"可见。"""
    walk = getattr(tpl, "walk", None)
    jitter = TILE_SIZE * 0.25
    if walk is not None and walk.enabled and walk.tiles:
        tile_id = walk.tiles[step % len(walk.tiles)]
        cx, cy = _tile_center(tile_id)
        return tile_id, cx + rng.uniform(-jitter, jitter), cy + rng.uniform(-jitter, jitter)
    tx, ty = _parse_tile(tpl.home_tile_id) if tpl.home_tile_id else (0, 0)
    candidates = [(tx, ty), (tx + 1, ty), (tx - 1, ty), (tx, ty + 1), (tx, ty - 1)]
    ntx, nty = rng.choice(candidates)
    cx, cy = _tile_center(f"tile_{ntx}_{nty}")
    return f"tile_{ntx}_{nty}", cx + rng.uniform(-jitter, jitter), cy + rng.uniform(-jitter, jitter)


def _parse_tile(tile_id: str) -> tuple[int, int]:
    try:
        _, x, y = tile_id.split("_")
        return int(x), int(y)
    except (ValueError, AttributeError):
        return 0, 0


class MoveScheduler:
    def __init__(
        self,
        *,
        registry: NpcRegistry,
        publisher: _PublisherLike,
        channel: str,
        tick_seconds: float,
        rng: random.Random | None = None,
        chooser: Chooser | None = None,
        clock: Callable[[], int] | None = None,
    ) -> None:
        self._registry = registry
        self._publisher = publisher
        self._channel = channel
        self._tick = tick_seconds
        self._rng: random.Random = rng if rng is not None else random.Random()
        self._chooser: Chooser = chooser if chooser is not None else default_chooser
        self._clock = clock if clock is not None else (lambda: int(time.time() * 1000))
        self._step = 0
        self._last_pos: dict[str, tuple[str, float, float]] = {}

    def tick_once(self) -> None:
        """每个 enabled NPC（有 home_tile_id）发布一条 npc_moved。"""
        for tpl in self._registry.list_enabled():
            if not tpl.home_tile_id:
                continue
            current = self._last_pos.get(tpl.npc_id)
            tile_id, x, y = self._chooser(tpl, current, self._step, self._rng)
            payload = {
                "npc_id": tpl.npc_id,
                "tile_id": tile_id,
                "x": round(float(x), 1),
                "y": round(float(y), 1),
                "ts_ms": self._clock(),
            }
            self._last_pos[tpl.npc_id] = (tile_id, float(x), float(y))
            try:
                self._publisher.publish(self._channel, json.dumps(payload))
            except Exception as e:  # noqa: BLE001
                logger.warning(
                    "npc move publish failed",
                    extra={"npc_id": tpl.npc_id, "err": str(e)},
                )
        self._step += 1

    async def run(self, stop: asyncio.Event) -> None:
        """长寿命 task。stop.set() 后下一次 tick 退出（与 SayScheduler 同款）。"""
        logger.info("move_scheduler starting", extra={"tick_seconds": self._tick})
        while not stop.is_set():
            try:
                self.tick_once()
            except Exception as e:
                logger.exception("npc move tick exception", extra={"err": str(e)})
            try:
                await asyncio.wait_for(stop.wait(), timeout=self._tick)
            except TimeoutError:
                pass
        logger.info("move_scheduler stopped")