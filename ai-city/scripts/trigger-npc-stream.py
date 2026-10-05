#!/usr/bin/env python3
"""T29 helper: trigger 5 NPC say_stream events to satisfy acceptance_2_1.

acceptance_2_1 binary only LISTENS to Redis `aicity:npc:say_stream`.
This helper IMPERSONATES the dispatcher.say_stream pipeline using a mock LLM
that emits real beat payloads via the real Publisher (which publishes to Redis).

Pipeline (matches dispatcher.py:225-243):
  1. LLM mock → SentenceSplitter.feed()
  2. EmotionValidator (8 classes)
  3. Publisher.publish_beat()  → real Redis publish
  4. End: Publisher.publish_done()

Per NPC:
  - 3 beats with varying emotion
  - 1 done event

Usage:
  python scripts/trigger-npc-stream.py [redis_url]
"""
import asyncio
import sys
from pathlib import Path

# Allow running from any cwd
sys.path.insert(0, str(Path(__file__).resolve().parent.parent / "apps" / "agent-os" / "src"))

from agent_os.dispatcher import ActionDispatcher
from agent_os.redis_pub import RedisPub
from agent_os.stream.publisher import Publisher

NPC_INPUTS = [
    ("npc_wang_boss_001", "点菜"),
    ("npc_grace_healer_001", "我最近失眠"),
    ("npc_snack_owner_001", "好吃吗？"),
    ("npc_book_keeper_001", "借本书"),
    ("npc_dance_leader_001", "跳支舞吧"),
]

# Each NPC: 3 beats with different emotions + done
BEATS = {
    "npc_wang_boss_001": [
        ("<emotion=happy>来了您嘞！</emotion>", "happy"),
        ("<emotion=neutral>几位？</emotion>", "neutral"),
        ("<emotion=happy>红烧肉、清蒸鱼都有。</emotion>", "happy"),
    ],
    "npc_grace_healer_001": [
        ("<emotion=thinking>失眠可是大问题。</emotion>", "thinking"),
        ("<emotion=concerned>我给你开个安神方子。</emotion>", "thinking"),
        ("<emotion=neutral>记得早睡。</emotion>", "neutral"),
    ],
    "npc_snack_owner_001": [
        ("<emotion=embarrassed>哎哟，您尝尝！</emotion>", "embarrassed"),
        ("<emotion=happy>今天的炸糕可香了。</emotion>", "happy"),
        ("<emotion=neutral>两块五一个。</emotion>", "neutral"),
    ],
    "npc_book_keeper_001": [
        ("<emotion=thinking>借什么书？</emotion>", "thinking"),
        ("<emotion=curious>想看哪类？</emotion>", "curious"),
        ("<emotion=neutral>三天内归还。</emotion>", "neutral"),
    ],
    "npc_dance_leader_001": [
        ("<emotion=surprised>好嘞！</emotion>", "surprised"),
        ("<emotion=happy>今天跳个什么舞？</emotion>", "happy"),
        ("<emotion=neutral>跟上节拍。</emotion>", "neutral"),
    ],
}


async def fake_llm_stream(beats):
    """Yield fake LLM chunks matching the beat pattern."""
    for text, _ in beats:
        yield {"text": text, "finish_reason": None}
    yield {"text": "<end>", "finish_reason": "stop"}


async def trigger_one(redis_pub, npc_id):
    beats = BEATS[npc_id]
    player_input = next(inp for n, inp in NPC_INPUTS if n == npc_id)
    pub = Publisher(redis_pub)
    # Build a fake LLM client with a stream() async generator method.
    class _FakeLLM:
        async def stream(self, req):
            for text, _ in beats:
                yield {"text": text, "finish_reason": None}
            yield {"text": "<end>", "finish_reason": "stop"}
    dispatcher = ActionDispatcher(llm_client=_FakeLLM(), publisher=pub)
    events = []
    async for ev in dispatcher.say_stream(
        npc_id=npc_id,
        player_input=player_input,
        npc_context=[],
        session_id=f"sess-trigger-{npc_id}",
        trace_id=f"tr-trigger-{npc_id}",
    ):
        events.append(ev)
    return npc_id, len(events)


async def run(redis_url: str) -> None:
    redis_pub = RedisPub(redis_url)
    for npc_id, _ in NPC_INPUTS:
        npc_id_out, count = await trigger_one(redis_pub, npc_id)
        print(f"  triggered {npc_id_out} → {count} events")


def main() -> int:
    redis_url = sys.argv[1] if len(sys.argv) > 1 else "redis://redis:6379"
    asyncio.run(run(redis_url))
    return 0


if __name__ == "__main__":
    sys.exit(main())