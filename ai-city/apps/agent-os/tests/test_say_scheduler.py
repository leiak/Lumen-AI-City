"""SayScheduler 每 tick 给 enabled NPC 发一句，主动 say 携带 talk_tree root options。"""
import asyncio
import textwrap
from pathlib import Path

import pytest

from agent_os.npc_registry import NpcRegistry
from agent_os.say_scheduler import SayScheduler


class FakeDispatcher:
    def __init__(self):
        self.calls: list[dict] = []

    async def say(self, npc_id: str, text: str, **kwargs) -> None:
        self.calls.append({"npc_id": npc_id, "text": text, **kwargs})


def _write_yaml(tmp: Path, name: str, body: str) -> Path:
    p = tmp / name
    p.write_text(textwrap.dedent(body), encoding="utf-8")
    return p


@pytest.mark.asyncio
async def test_tick_emits_one_say_per_enabled_npc(tmp_path: Path):
    _write_yaml(tmp_path, "wang_boss.yaml", """\
        npc_id: npc_wang_boss_001
        enabled: true
        say:
          greeting:
            - "来了您嘞！"
    """)
    _write_yaml(tmp_path, "lihua.yaml", """\
        npc_id: npc_lihua_002
        enabled: false
        say:
          greeting:
            - "我不在"
    """)
    reg = NpcRegistry(tmp_path)
    disp = FakeDispatcher()
    sched = SayScheduler(registry=reg, dispatcher=disp, tick_seconds=0.01)  # type: ignore[arg-type]

    await sched.tick_once()
    assert [c["npc_id"] for c in disp.calls] == ["npc_wang_boss_001"]
    assert disp.calls[0]["text"] == "来了您嘞！"
    # 无 talk_tree -> options 不传入（dispatcher 兜底为 []）
    assert disp.calls[0]["options"] is None


@pytest.mark.asyncio
async def test_tick_carries_talk_tree_root_options(tmp_path: Path):
    _write_yaml(tmp_path, "wang_boss.yaml", """\
        npc_id: npc_wang_boss_001
        enabled: true
        say:
          greeting:
            - "来了您嘞！"
        talk_tree:
          initial: root
          nodes:
            root:
              say: "来了您嘞！"
              options:
                - {id: ask_food, text: "有什么招牌菜？"}
                - {id: leave, text: "先走了"}
            ask_food:
              say: "炸酱面。"
              options: []
            leave:
              say: "慢走。"
              options: []
    """)
    reg = NpcRegistry(tmp_path)
    disp = FakeDispatcher()
    sched = SayScheduler(registry=reg, dispatcher=disp, tick_seconds=0.01)  # type: ignore[arg-type]

    await sched.tick_once()
    assert disp.calls[0]["npc_id"] == "npc_wang_boss_001"
    assert disp.calls[0]["options"] == [
        {"id": "ask_food", "text": "有什么招牌菜？"},
        {"id": "leave", "text": "先走了"},
    ]


@pytest.mark.asyncio
async def test_tick_no_enabled_skips_silently(tmp_path: Path):
    _write_yaml(tmp_path, "lihua.yaml", """\
        npc_id: npc_lihua_002
        enabled: false
    """)
    reg = NpcRegistry(tmp_path)
    disp = FakeDispatcher()
    sched = SayScheduler(registry=reg, dispatcher=disp, tick_seconds=0.01)  # type: ignore[arg-type]
    await sched.tick_once()
    assert disp.calls == []


@pytest.mark.asyncio
async def test_run_stops_on_event(tmp_path: Path):
    _write_yaml(tmp_path, "wang_boss.yaml", """\
        npc_id: npc_wang_boss_001
        enabled: true
        say:
          greeting:
            - "来了您嘞！"
    """)
    reg = NpcRegistry(tmp_path)
    disp = FakeDispatcher()
    sched = SayScheduler(registry=reg, dispatcher=disp, tick_seconds=0.01)  # type: ignore[arg-type]

    stop = asyncio.Event()
    task = asyncio.create_task(sched.run(stop))
    await asyncio.sleep(0.05)  # 至少跑 2 tick
    stop.set()
    await asyncio.wait_for(task, timeout=1.0)
    assert len(disp.calls) >= 1
class FakeListener:
    def __init__(self, occupied: set[str] | None = None):
        self._occupied = set(occupied or [])

    def players_in_tile(self, tile_id: str):
        return [object()] if tile_id in self._occupied else []


@pytest.mark.asyncio
async def test_tick_skips_when_player_at_home(tmp_path: Path):
    """Sprint 13：home tile 有玩家时 tick 不广播 greeting（交给 WelcomeEngine）。"""
    _write_yaml(tmp_path, "wang_boss.yaml", """\
        npc_id: npc_wang_boss_001
        enabled: true
        home_tile_id: tile_0_0
        say:
          greeting:
            - "来了您嘞！"
    """)
    reg = NpcRegistry(tmp_path)
    disp = FakeDispatcher()
    listener = FakeListener({"tile_0_0"})
    sched = SayScheduler(
        registry=reg, dispatcher=disp, tick_seconds=0.01, listener=listener  # type: ignore[arg-type]
    )
    await sched.tick_once()
    assert disp.calls == []


@pytest.mark.asyncio
async def test_tick_greets_when_no_player_at_home(tmp_path: Path):
    """home 无玩家（或玩家在别处 / 无 listener）时仍广播 greeting 兜底。"""
    _write_yaml(tmp_path, "wang_boss.yaml", """\
        npc_id: npc_wang_boss_001
        enabled: true
        home_tile_id: tile_0_0
        say:
          greeting:
            - "来了您嘞！"
    """)
    reg = NpcRegistry(tmp_path)
    for listener in (FakeListener(), FakeListener({"tile_1_0"})):
        disp = FakeDispatcher()
        sched = SayScheduler(
            registry=reg, dispatcher=disp, tick_seconds=0.01, listener=listener  # type: ignore[arg-type]
        )
        await sched.tick_once()
        assert [c["npc_id"] for c in disp.calls] == ["npc_wang_boss_001"]

@pytest.mark.asyncio
async def test_tick_falls_back_to_default_say():
    """无 greeting 时 tick 兜底用 talk_tree.default_say。"""
    from agent_os.npc_registry import NpcTemplate, Say, TalkTree

    tpl = NpcTemplate(npc_id="npc_wang_boss_001", enabled=True)
    tpl.say = Say(greeting=[], welcome=[])
    tpl.talk_tree = TalkTree(initial="root", default_say="您先看着，我忙完这茬儿再说。")

    class FakeRegistry:
        def list_enabled(self):
            return [tpl]

    disp = FakeDispatcher()
    sched = SayScheduler(registry=FakeRegistry(), dispatcher=disp, tick_seconds=0.01)  # type: ignore[arg-type]
    await sched.tick_once()
    assert [c["npc_id"] for c in disp.calls] == ["npc_wang_boss_001"]
    assert disp.calls[0]["text"] == "您先看着，我忙完这茬儿再说。"


@pytest.mark.asyncio
async def test_tick_no_greeting_no_default_skips():
    """无 greeting 且无 default_say 时不广播（不抛 IndexError）。"""
    from agent_os.npc_registry import NpcTemplate, Say

    tpl = NpcTemplate(npc_id="npc_wang_boss_001", enabled=True)
    tpl.say = Say(greeting=[], welcome=[])

    class FakeRegistry:
        def list_enabled(self):
            return [tpl]

    disp = FakeDispatcher()
    sched = SayScheduler(registry=FakeRegistry(), dispatcher=disp, tick_seconds=0.01)  # type: ignore[arg-type]
    await sched.tick_once()
    assert disp.calls == []
