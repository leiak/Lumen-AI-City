"""Phase A.3 — EmotionSettings.OCEAN_BIAS_ENABLED kill switch tests."""
import pytest

from agent_os.emotion.settings import EmotionSettings


def test_default_ocean_bias_enabled_true(monkeypatch):
    monkeypatch.delenv("OCEAN_BIAS_ENABLED", raising=False)
    assert EmotionSettings.from_env().ocean_bias_enabled is True


def test_occean_bias_disabled_via_env(monkeypatch):
    monkeypatch.setenv("OCEAN_BIAS_ENABLED", "false")
    assert EmotionSettings.from_env().ocean_bias_enabled is False


def test_occean_bias_enabled_via_env_one(monkeypatch):
    monkeypatch.setenv("OCEAN_BIAS_ENABLED", "1")
    assert EmotionSettings.from_env().ocean_bias_enabled is True


def test_occean_bias_enabled_via_env_yes(monkeypatch):
    monkeypatch.setenv("OCEAN_BIAS_ENABLED", "yes")
    assert EmotionSettings.from_env().ocean_bias_enabled is True


def test_ocean_bias_malformed_defaults_false(monkeypatch):
    # Lenient parsing (no exception thrown), matches existing inject_enabled pattern:
    #   "true"/"1"/"yes" -> True; everything else -> False.
    # Spec default True is achieved only when env var is absent.
    monkeypatch.setenv("OCEAN_BIAS_ENABLED", "garbage")
    assert EmotionSettings.from_env().ocean_bias_enabled is False