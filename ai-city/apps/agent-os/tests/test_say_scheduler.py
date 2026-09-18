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