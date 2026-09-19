"""MoveScheduler：每 tick 给每个 enabled NPC 发一条 npc_moved（可用 mock 离线测试）。"""
import asyncio
import json
import random

import pytest

from agent_os.move_scheduler import MoveScheduler, default_chooser
from agent_os.npc_registry import NpcTemplate, Walk


class FakeRegistry:
    def __init__(self, templates: list[NpcTemplate]):
        self._templates = templates

    def list_enabled(self) -> list[NpcTemplate]:
        # 与真实 NpcRegistry 一致：enabled=False 不下发
        return [t for t in self._templates if t.enabled]


class FakePublisher:
    def __init__(self):
        self.calls: list[tuple[str, str]] = []

    def publish(self, channel: str, payload: str) -> None:
        self.calls.append((channel, payload))


def _tpl(npc_id="npc_wang_boss_001", enabled=True, home="tile_0_0") -> NpcTemplate:
    return NpcTemplate(npc_id=npc_id, enabled=enabled, home_tile_id=home)


def _clock_seq():
    n = [1_000_000]
    def clock() -> int:
        n[0] += 1000
        return n[0]
    return clock


def test_tick_publishes_one_npc_moved_per_enabled():
    pub = FakePublisher()
    sched = MoveScheduler(
        registry=FakeRegistry([_tpl(), _tpl("npc_lihua_001", enabled=True, home="tile_1_0")]),
        publisher=pub,  # type: ignore[arg-type]
        channel="aicity:npc_moved",
        tick_seconds=30.0,
        rng=random.Random(7),
        clock=_clock_seq(),
    )
    sched.tick_once()
    assert len(pub.calls) == 2
    for channel, payload in pub.calls:
        assert channel == "aicity:npc_moved"
        msg = json.loads(payload)
        assert set(msg) == {"npc_id", "tile_id", "x", "y", "ts_ms"}
        assert msg["npc_id"] in {"npc_wang_boss_001", "npc_lihua_001"}
        assert isinstance(msg["tile_id"], str)
        assert isinstance(msg["x"], float)
        assert isinstance(msg["y"], float)
        assert isinstance(msg["ts_ms"], int)


def test_tick_skips_disabled_and_homeless():
    pub = FakePublisher()
    sched = MoveScheduler(
        registry=FakeRegistry([
            _tpl(),                                        # enabled → 发
            _tpl("npc_disabled_1", enabled=False),          # disabled → 不发
            NpcTemplate(npc_id="npc_no_home", enabled=True),  # 无 home → 不发
        ]),
        publisher=pub,  # type: ignore[arg-type]
        channel="aicity:npc_moved",
        tick_seconds=30.0,
        rng=random.Random(1),
    )
    sched.tick_once()
    assert [json.loads(p)["npc_id"] for _, p in pub.calls] == [
        "npc_wang_boss_001"
    ]


def test_tick_tracks_last_position_for_continuity():
    pub = FakePublisher()
    rng = random.Random(3)
    sched = MoveScheduler(
        registry=FakeRegistry([_tpl()]),
        publisher=pub,  # type: ignore[arg-type]
        channel="aicity:npc_moved",
        tick_seconds=30.0,
        rng=rng,
        clock=_clock_seq(),
    )
    sched.tick_once()
    sched.tick_once()
    assert len(pub.calls) == 2
    first = json.loads(pub.calls[0][1])
    second = json.loads(pub.calls[1][1])
    # 起始位置被记录后，第二次 chooser 会基于同一对象继续（这里只断言两条都发出且 ts_ms 递增）
    assert first["ts_ms"] < second["ts_ms"]


def test_default_chooser_stays_within_home_plus_minus_one():
    rng = random.Random(11)
    tpl = _tpl(home="tile_0_0")
    allowed = {
        ("0", "0"), ("1", "0"), ("-1", "0"), ("0", "1"), ("0", "-1"),
    }
    for _ in range(200):
        tile_id, x, y = default_chooser(tpl, None, 0, rng)
        _, nx, ny = tile_id.split("_")
        assert (nx, ny) in allowed, f"leaked outside home±1: {tile_id}"
        # 坐标应在目标 tile 的 [center±37.5] 内（center±0.25*100 抖动）
        cx_target = int(nx) * 100 + 50
        cy_target = int(ny) * 100 + 50
        assert abs(x - cx_target) < 37.5, f"x out of bounds: {x} center={cx_target}"
        assert abs(y - cy_target) < 37.5, f"y out of bounds: {y} center={cy_target}"


def test_chooser_injectable():
    """自定义 chooser 决定移动目标（供测试/后续模板步法复用）。"""
    pub = FakePublisher()
    def chooser(tpl, current, step, rng) -> tuple[str, float, float]:
        return "tile_1_0", 150.0, 50.0
    sched = MoveScheduler(
        registry=FakeRegistry([_tpl()]),
        publisher=pub,  # type: ignore[arg-type]
        channel="aicity:npc_moved",
        tick_seconds=30.0,
        chooser=chooser,
    )
    sched.tick_once()
    msg = json.loads(pub.calls[0][1])
    assert msg["tile_id"] == "tile_1_0"
    assert msg["x"] == 150.0
    assert msg["y"] == 50.0


@pytest.mark.asyncio
async def test_run_stops_on_event():
    pub = FakePublisher()
    sched = MoveScheduler(
        registry=FakeRegistry([_tpl()]),
        publisher=pub,  # type: ignore[arg-type]
        channel="aicity:npc_moved",
        tick_seconds=0.01,
        rng=random.Random(5),
    )
    stop = asyncio.Event()
    task = asyncio.create_task(sched.run(stop))
    await asyncio.sleep(0.04)  # 至少跑 2 tick
    stop.set()
    await asyncio.wait_for(task, timeout=1.0)
    assert len(pub.calls) >= 1

def test_default_chooser_follows_template_walk():
    """模板带 walk.tiles 时，chooser 按序列循环走（3 步走法），坐标落在对应 tile 中心附近。"""
    rng = random.Random(2)
    tpl = _tpl(home="tile_0_0")
    tpl.walk = Walk(enabled=True, tiles=["tile_0_0", "tile_1_0", "tile_-1_0"])
    results = [default_chooser(tpl, None, step, rng) for step in range(6)]
    tiles = [r[0] for r in results]
    assert tiles == ["tile_0_0", "tile_1_0", "tile_-1_0", "tile_0_0", "tile_1_0", "tile_-1_0"]
    for tile_id, x, y in results:
        _, nx, ny = tile_id.split("_")
        cx = int(nx) * 100 + 50
        cy = int(ny) * 100 + 50
        assert abs(x - cx) < 37.5, f"x={x} off tile {tile_id}"
        assert abs(y - cy) < 37.5, f"y={y} off tile {tile_id}"


def test_default_chooser_without_walk_stays_random_neighbor():
    """无 walk 模板回退为 home±1 随机邻格（原有行为不回归）。"""
    rng = random.Random(9)
    tpl = _tpl(home="tile_0_0")  # walk=None
    allowed = {("0", "0"), ("1", "0"), ("-1", "0"), ("0", "1"), ("0", "-1")}
    for _ in range(50):
        tile_id, _, _ = default_chooser(tpl, None, 0, rng)
        _, nx, ny = tile_id.split("_")
        assert (nx, ny) in allowed
