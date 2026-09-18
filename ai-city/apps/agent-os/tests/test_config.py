"""config defaults — Sprint 12 减面后只剩本范围需要的字段。"""
import os

from agent_os.config import Settings


def test_http_port_defaults_to_8084():
    os.environ.pop("HTTP_PORT", None)
    s = Settings()
    assert s.http_port == 8084


def test_redis_url_default():
    os.environ.pop("REDIS_URL", None)
    s = Settings()
    assert s.redis_url == "redis://localhost:6379/0"


def test_npc_dialogue_channel_default():
    os.environ.pop("REDIS_CHANNEL_NPC_DIALOGUE", None)
    s = Settings()
    assert s.redis_channel_npc_dialogue == "aicity:npc_dialogue"


def test_player_moved_channel_default():
    os.environ.pop("REDIS_CHANNEL_PLAYER_MOVED", None)
    s = Settings()
    assert s.redis_channel_player_moved == "aicity:player:moved"


def test_say_tick_seconds_default():
    os.environ.pop("SAY_TICK_SECONDS", None)
    s = Settings()
    assert s.say_tick_seconds == 5.0
