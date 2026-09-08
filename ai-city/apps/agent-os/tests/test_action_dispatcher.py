"""ActionDispatcher.say() 构造 envelope + publish。"""
import json

import pytest

from agent_os.action_dispatcher import ActionDispatcher
from agent_os.redis_pub import RedisStats


class FakeRedis:
    def __init__(self):
        self.calls: list[tuple[str, str]] = []
        self.stats = RedisStats()

    async def publish(self, channel: str, payload: str) -> None:
        self.calls.append((channel, payload))

    def stats_(self):
        return self.stats


@pytest.mark.asyncio
async def test_say_minimal_publishes_envelope():
    fake = FakeRedis()
    d = ActionDispatcher(fake, channel="aicity:npc_dialogue")  # type: ignore[arg-type]
    await d.say("npc_wang_boss_001", "来了您嘞！")
    assert len(fake.calls) == 1
    channel, payload = fake.calls[0]
    assert channel == "aicity:npc_dialogue"
    env = json.loads(payload)
    assert env["type"] == "npc_dialogue"
    assert env["payload"]["npc_id"] == "npc_wang_boss_001"
    assert env["payload"]["say"] == "来了您嘞！"
    assert env["payload"]["options"] == []  # 空数组不是 null
    assert env["trace_id"]  # uuid 字符串非空
    assert env["ts_ms"] > 0


@pytest.mark.asyncio
async def test_say_with_options_and_reply_to():
    fake = FakeRedis()
    d = ActionDispatcher(fake, channel="aicity:npc_dialogue")  # type: ignore[arg-type]
    await d.say(
        "npc_wang_boss_001",
        "回您一句。",
        player_id="p-1",
        tile_id="tile_0_0",
        options=[{"id": "ok", "text": "好的"}],
        reply_to_choice_id="ask_business",
    )
    env = json.loads(fake.calls[0][1])
    p = env["payload"]
    assert p["player_id"] == "p-1"
    assert p["tile_id"] == "tile_0_0"
    assert p["options"] == [{"id": "ok", "text": "好的"}]
    assert p["reply_to_choice_id"] == "ask_business"


@pytest.mark.asyncio
async def test_say_publish_failure_does_not_raise():
    class BadRedis:
        async def publish(self, channel, payload):
            raise RuntimeError("simulated")

    d = ActionDispatcher(BadRedis(), channel="aicity:npc_dialogue")  # type: ignore[arg-type]
    # 不抛：fire-and-forget
    await d.say("npc_wang_boss_001", "x")