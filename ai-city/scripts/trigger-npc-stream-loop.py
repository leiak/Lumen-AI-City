#!/usr/bin/env python3
"""T29 helper loop: continuously trigger 5 NPC say_stream events.

acceptance_2_1 listens per NPC for 8s window; this loop publishes 5 NPC events
every 3s so the listener always has fresh beats to capture.

Stop with Ctrl-C (or SIGTERM).
"""
import asyncio
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent / "apps" / "agent-os" / "src"))

from agent_os.dispatcher import ActionDispatcher
from agent_os.redis_pub import RedisPub
from agent_os.stream.publisher import Publisher

NPC_IDS = [
    "npc_wang_boss_001",
    "npc_grace_healer_001",
    "npc_snack_owner_001",
    "npc_book_keeper_001",
    "npc_dance_leader_001",
]

BEATS = {
    "npc_wang_boss_001": [
        "<emotion=happy>来了您嘞！</emotion>",
        "<emotion=neutral>几位？</emotion>",
        "<emotion=happy>红烧肉、清蒸鱼都有。</emotion>",
    ],
    "npc_grace_healer_001": [
        "<emotion=thinking>失眠可是大问题。</emotion>",
        "<emotion=thinking>我给你开个安神方子。</emotion>",
        "<emotion=neutral>记得早睡。</emotion>",
    ],
    "npc_snack_owner_001": [
        "<emotion=embarrassed>哎哟，您尝尝！</emotion>",
        "<emotion=happy>今天的炸糕可香了。</emotion>",
        "<emotion=neutral>两块五一个。</emotion>",
    ],
    "npc_book_keeper_001": [
        "<emotion=thinking>借什么书？</emotion>",
        "<emotion=curious>想看哪类？</emotion>",
        "<emotion=neutral>三天内归还。</emotion>",
    ],
    "npc_dance_leader_001": [
        "<emotion=surprised>好嘞！</emotion>",
        "<emotion=happy>今天跳个什么舞？</emotion>",
        "<emotion=neutral>跟上节拍。</emotion>",
    ],
}


class _FakeLLM:
    def __init__(self, beats):
        self._beats = beats

    async def stream(self, req):
        for text in self._beats:
            yield {"text": text, "finish_reason": None}
        yield {"text": "<end>", "finish_reason": "stop"}


async def trigger_one(redis_pub, npc_id):
    pub = Publisher(redis_pub)
    dispatcher = ActionDispatcher(llm_client=_FakeLLM(BEATS[npc_id]), publisher=pub)
    count = 0
    async for _ in dispatcher.say_stream(
        npc_id=npc_id,
        player_input="trigger",
        npc_context=[],
        session_id=f"sess-loop-{npc_id}",
        trace_id=f"tr-loop-{npc_id}",
    ):
        count += 1
    return count


async def loop_publish(redis_url, interval_s):
    redis_pub = RedisPub(redis_url)
    iteration = 0
    while True:
        iteration += 1
        print(f"[iter {iteration}] publishing 5 NPC streams...", flush=True)
        for npc_id in NPC_IDS:
            count = await trigger_one(redis_pub, npc_id)
            print(f"  {npc_id} → {count}", flush=True)
        await asyncio.sleep(interval_s)


def main():
    redis_url = sys.argv[1] if len(sys.argv) > 1 else "redis://redis:6379"
    interval = float(sys.argv[2]) if len(sys.argv) > 2 else 3.0
    try:
        asyncio.run(loop_publish(redis_url, interval))
    except KeyboardInterrupt:
        print("\nstopped", flush=True)


if __name__ == "__main__":
    main()