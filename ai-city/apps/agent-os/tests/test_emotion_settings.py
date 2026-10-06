"""Emotion settings (env vars) tests."""
import pytest

from agent_os.emotion.settings import EmotionSettings


def test_defaults_when_no_env(monkeypatch):
    for k in ("EMOTION_TAU_PLAYER_SECONDS", "EMOTION_TAU_GLOBAL_SECONDS",
              "EMOTION_PLAYER_LIMIT", "EMOTION_GLOBAL_LIMIT",
              "EMOTION_INJECT_ENABLED"):
        monkeypatch.delenv(k, raising=False)
    s = EmotionSettings.from_env()
    assert s.tau_player_seconds == 7200.0
    assert s.tau_global_seconds == 86400.0
    assert s.player_limit == 50
    assert s.global_limit == 100
    assert s.inject_enabled is True


def test_tau_player_override(monkeypatch):
    monkeypatch.setenv("EMOTION_TAU_PLAYER_SECONDS", "3600")
    s = EmotionSettings.from_env()
    assert s.tau_player_seconds == 3600.0


def test_tau_global_override(monkeypatch):
    monkeypatch.setenv("EMOTION_TAU_GLOBAL_SECONDS", "172800")
    s = EmotionSettings.from_env()
    assert s.tau_global_seconds == 172800.0


def test_inject_enabled_false(monkeypatch):
    monkeypatch.setenv("EMOTION_INJECT_ENABLED", "false")
    s = EmotionSettings.from_env()
    assert s.inject_enabled is False


def test_invalid_tau_raises(monkeypatch):
    monkeypatch.setenv("EMOTION_TAU_PLAYER_SECONDS", "0")
    with pytest.raises(ValueError):
        EmotionSettings.from_env()


def test_invalid_limit_raises(monkeypatch):
    monkeypatch.setenv("EMOTION_PLAYER_LIMIT", "-1")
    with pytest.raises(ValueError):
        EmotionSettings.from_env()
