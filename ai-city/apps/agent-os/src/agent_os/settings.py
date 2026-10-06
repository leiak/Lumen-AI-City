"""Runtime settings (env-driven)."""
import os


def get_redis_channel_npc_say_stream() -> str:
    """Return the Redis channel for NPC stream beats.

    Resolves from env `REDIS_CHANNEL_NPC_SAY_STREAM`, falling back to the
    default `aicity:npc:say_stream` if the env var is unset, empty, or
    whitespace-only.
    """
    val = os.getenv("REDIS_CHANNEL_NPC_SAY_STREAM")
    if val is None or not val.strip():
        return "aicity:npc:say_stream"
    return val.strip()
