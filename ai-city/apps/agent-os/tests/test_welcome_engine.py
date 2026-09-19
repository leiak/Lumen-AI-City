"""WelcomeEngine：玩家首入 NPC home tile 触发一次 welcome（可用 mock 离线测试）。"""
import json

import pytest

from agent_os.npc_registry import DialogOption, NpcTemplate, Say, TalkNode, TalkTree
from agent_os.welcome_engine import WelcomeEngine


class FakeRegistry:
    def __init__(self, templates: list[NpcTemplate]):
        self._templates = templates

    def list_enabled(self) -> list[NpcTemplate]:
        return self._templates


class FakeDispatcher:
    def __init__(self):
        self.calls: list[dict] = []

    async def say(self, npc_id, text, **kwargs) -> None:
        self.calls.append({"npc_id": npc_id, "text": text, **kwargs})


def _template(npc_id="npc_wang_boss_001", home="tile_0_0", welcome=None,
              greeting=None, with_tree=False, default_say=""):
    tpl = NpcTemplate(npc_id=npc_id, enabled=True, home_tile_id=home)
    if greeting is None:
        greeting = ["来了您嘞！"]
    tpl.say = Say(
        welcome=list(welcome or []),
        greeting=list(greeting),
        default_reply="",
    )
    if default_say:
        tpl.talk_tree.default_say = default_say
    if with_tree:
        tpl.talk_tree = TalkTree(
            initial="root",
            nodes={
                "root": TalkNode(
                    say="来了您嘞！",
                    options=[DialogOption(id="ask_food", text="有什么菜？")],
                )
            },
            default_say=default_say,
        )
    return tpl


def _moved(player="p1", tile="tile_0_0"):
    return json.dumps(
        {
            "player_id": player,
            "tile_id": tile,
            "x": 50.0,
            "y": 50.0,
            "ts_ms": 1,
        }
    )


@pytest.mark.asyncio
async def test_welcome_triggers_once():
    disp = FakeDispatcher()
    engine = WelcomeEngine(
        registry=FakeRegistry([_template(welcome=["欢迎光临！"])]),
        dispatcher=disp,  # type: ignore[arg-type]
    )
    await engine.handle_payload(_moved("p1", "tile_0_0"))
    await engine.handle_payload(_moved("p1", "tile_0_0"))  # 重复移动不再触发
    assert len(disp.calls) == 1
    assert disp.calls[0]["text"] == "欢迎光临！"
    assert disp.calls[0]["player_id"] == "p1"
    assert engine.listener.is_welcomed("p1") is True


@pytest.mark.asyncio
async def test_welcome_only_in_home_tile():
    disp = FakeDispatcher()
    engine = WelcomeEngine(
        registry=FakeRegistry([_template(welcome=["欢迎光临！"])]),
        dispatcher=disp,  # type: ignore[arg-type]
    )
    await engine.handle_payload(_moved("p1", "tile_1_0"))
    assert disp.calls == []
    assert engine.listener.is_welcomed("p1") is False


@pytest.mark.asyncio
async def test_welcome_falls_back_to_greeting():
    disp = FakeDispatcher()
    engine = WelcomeEngine(
        registry=FakeRegistry([_template(welcome=[], greeting=["来了您嘞！"])]),
        dispatcher=disp,  # type: ignore[arg-type]
    )
    await engine.handle_payload(_moved("p1", "tile_0_0"))
    assert len(disp.calls) == 1
    assert disp.calls[0]["text"] == "来了您嘞！"


@pytest.mark.asyncio
async def test_welcome_falls_back_to_default_say():
    disp = FakeDispatcher()
    engine = WelcomeEngine(
        registry=FakeRegistry(
            [_template(welcome=[], greeting=[], default_say="您先看着，我忙完这茬儿再说。")]
        ),
        dispatcher=disp,  # type: ignore[arg-type]
    )
    await engine.handle_payload(_moved("p1", "tile_0_0"))
    assert len(disp.calls) == 1
    assert disp.calls[0]["text"] == "您先看着，我忙完这茬儿再说。"


@pytest.mark.asyncio
async def test_welcome_no_lines_does_not_spoke():
    disp = FakeDispatcher()
    engine = WelcomeEngine(
        registry=FakeRegistry([_template(welcome=[], greeting=[], default_say="")]),
        dispatcher=disp,  # type: ignore[arg-type]
    )
    await engine.handle_payload(_moved("p1", "tile_0_0"))
    assert disp.calls == []
    assert engine.listener.is_welcomed("p1") is False


@pytest.mark.asyncio
async def test_welcome_carries_root_options():
    disp = FakeDispatcher()
    engine = WelcomeEngine(
        registry=FakeRegistry([_template(welcome=["欢迎光临！"], with_tree=True)]),
        dispatcher=disp,  # type: ignore[arg-type]
    )
    await engine.handle_payload(_moved("p1", "tile_0_0"))
    assert len(disp.calls) == 1
    assert disp.calls[0]["options"] == [{"id": "ask_food", "text": "有什么菜？"}]


@pytest.mark.asyncio
async def test_invalid_payload_ignored():
    disp = FakeDispatcher()
    engine = WelcomeEngine(
        registry=FakeRegistry([_template(welcome=["欢迎光临！"])]),
        dispatcher=disp,  # type: ignore[arg-type]
    )
    await engine.handle_payload("not json")
    assert disp.calls == []


@pytest.mark.asyncio
async def test_welcomed_skips_after_first():
    disp = FakeDispatcher()
    engine = WelcomeEngine(
        registry=FakeRegistry([_template(welcome=["欢迎光临！"])]),
        dispatcher=disp,  # type: ignore[arg-type]
    )
    await engine.handle_payload(_moved("p1", "tile_0_0"))
    await engine.handle_payload(_moved("p1", "tile_1_0"))  # 离开再进不重复
    await engine.handle_payload(_moved("p2", "tile_0_0"))  # 新玩家仍触发
    assert [c["player_id"] for c in disp.calls] == ["p1", "p2"]
