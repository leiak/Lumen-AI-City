"""Runtime settings (env-driven).

Plain os.getenv on purpose: keeps deps minimal and lets tests monkeypatch
the helper functions directly. Callers should invoke these at *call time*
(not import time) so env overrides take effect after process start.
"""
from __future__ import annotations

import os


def get_redis_channel_npc_say_stream() -> str:
    """Redis channel used for streaming NPC say beats.

    Override via env ``REDIS_CHANNEL_NPC_SAY_STREAM``. Default
    ``aicity:npc:say_stream`` matches the historical hardcoded value
    so existing subscribers keep working.
    """
    return os.getenv("REDIS_CHANNEL_NPC_SAY_STREAM", "aicity:npc:say_stream")