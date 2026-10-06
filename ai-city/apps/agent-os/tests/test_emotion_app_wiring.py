"""B2-T08 App wiring tests: emotion repository lifecycle + say_stream integration.

Verifies:
1. App lifespan creates EmotionRepository when PG_DSN is set.
2. App lifespan does NOT create EmotionRepository when PG_DSN is unset (graceful).
3. App lifespan closes the pool on shutdown (best-effort).
4. ``say_stream`` fetches both distributions when ``inject_enabled=True``.
5. ``say_stream`` skips fetch when ``inject_enabled=False``.
6. ``say_stream`` logs warning + falls back to ``None`` when fetch raises.
7. ``say_stream`` passes the fetched distributions to ``get_npc_stream_prompt``.
8. ``say_stream`` with ``emotion_repo=None`` yields pre-B2 behavior (no fetch).
"""
from __future__ import annotations

import asyncio
import logging
from pathlib import Path
from unittest.mock import AsyncMock, MagicMock, patch

import pytest
from fastapi.testclient import TestClient

from agent_os.dispatcher import ActionDispatcher
from agent_os.emotion.aggregate import EmotionDistribution


# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------


@pytest.fixture
def fake_repo() -> AsyncMock:
    """AsyncMock of EmotionRepository with empty default distributions."""
    repo = AsyncMock()
    empty = EmotionDistribution(weights={}, raw_counts={}, total_rows=0)
    repo.fetch_player_distribution.return_value = empty
    repo.fetch_global_distribution.return_value = empty
    return repo


@pytest.fixture
def fake_settings_enabled():
    """EmotionSettings with inject_enabled=True."""
    from agent_os.emotion.settings import EmotionSettings

    return EmotionSettings(
        tau_player_seconds=7200.0,
        tau_global_seconds=86400.0,
        player_limit=50,
        global_limit=100,
        inject_enabled=True,
    )


@pytest.fixture
def fake_settings_disabled():
    """EmotionSettings with inject_enabled=False."""
    from agent_os.emotion.settings import EmotionSettings

    return EmotionSettings(
        tau_player_seconds=7200.0,
        tau_global_seconds=86400.0,
        player_limit=50,
        global_limit=100,
        inject_enabled=False,
    )


@pytest.fixture
def templates_dir(tmp_path: Path) -> Path:
    f = tmp_path / "wang.yaml"
    f.write_text(
        "npc_id: npc_wang_boss_001\n"
        "enabled: true\n"
        "say:\n"
        "  greeting:\n"
        "    - '来了您嘞！'\n",
        encoding="utf-8",
    )
    return tmp_path


def _build_dispatcher_with_repo(repo, settings):
    """Build a minimal ActionDispatcher wired with emotion_repo + settings."""
    publisher = MagicMock()
    publisher.publish_beat = AsyncMock()
    publisher.publish_done = AsyncMock()
    return ActionDispatcher(
        llm_client=MagicMock(),
        publisher=publisher,
        emotion_repo=repo,
        emotion_settings=settings,
    )


# ---------------------------------------------------------------------------
# Test 1: emotion_repo=None → pre-B2 behavior (no fetch)
# ---------------------------------------------------------------------------


def test_say_stream_no_emotion_repo_preserves_pre_b2_behavior():
    """When emotion_repo is None, fetch methods are NEVER called (no DB I/O)."""

    async def fake_stream(req):
        yield {"text": "<emotion=happy>好</emotion>", "finish_reason": None}
        yield {"text": "<end>", "finish_reason": "stop"}

    publisher = MagicMock()
    publisher.publish_beat = AsyncMock()
    publisher.publish_done = AsyncMock()
    llm = MagicMock()
    llm.stream = fake_stream

    dispatcher = ActionDispatcher(llm_client=llm, publisher=publisher)
    assert dispatcher.emotion_repo is None

    async def run():
        async for _ in dispatcher.say_stream(
            npc_id="npc_wang_boss_001",
            player_input="点菜",
            npc_context=[],
            session_id="sess-1",
            trace_id="tr-1",
        ):
            pass

    asyncio.run(run())
    # Stream completed without error → pre-B2 behavior preserved
    publisher.publish_done.assert_called_once()


# ---------------------------------------------------------------------------
# Test 2: say_stream fetches both distributions when enabled
# ---------------------------------------------------------------------------


def test_say_stream_fetches_distributions_when_enabled(fake_repo, fake_settings_enabled):
    """When emotion_repo + inject_enabled=True, both fetch_* methods are called."""

    async def fake_stream(req):
        yield {"text": "<emotion=happy>好</emotion>", "finish_reason": None}
        yield {"text": "<end>", "finish_reason": "stop"}

    dispatcher = _build_dispatcher_with_repo(fake_repo, fake_settings_enabled)
    dispatcher.llm_client.stream = fake_stream

    async def run():
        async for _ in dispatcher.say_stream(
            npc_id="npc_wang_boss_001",
            player_input="点菜",
            npc_context=[],
            session_id="sess-2",
            trace_id="tr-2",
            player_id="player-1",
        ):
            pass

    asyncio.run(run())

    fake_repo.fetch_player_distribution.assert_awaited_once_with(
        "npc_wang_boss_001", "player-1",
    )
    fake_repo.fetch_global_distribution.assert_awaited_once_with("npc_wang_boss_001")


# ---------------------------------------------------------------------------
# Test 3: say_stream skips fetch when inject_enabled=False
# ---------------------------------------------------------------------------


def test_say_stream_skips_fetch_when_disabled(fake_repo, fake_settings_disabled):
    """When inject_enabled=False, neither fetch method is called."""

    async def fake_stream(req):
        yield {"text": "<emotion=happy>好</emotion>", "finish_reason": None}
        yield {"text": "<end>", "finish_reason": "stop"}

    dispatcher = _build_dispatcher_with_repo(fake_repo, fake_settings_disabled)
    dispatcher.llm_client.stream = fake_stream

    async def run():
        async for _ in dispatcher.say_stream(
            npc_id="npc_wang_boss_001",
            player_input="点菜",
            npc_context=[],
            session_id="sess-3",
            trace_id="tr-3",
            player_id="player-3",
        ):
            pass

    asyncio.run(run())

    fake_repo.fetch_player_distribution.assert_not_called()
    fake_repo.fetch_global_distribution.assert_not_called()


# ---------------------------------------------------------------------------
# Test 4: say_stream logs warning + falls back on fetch failure
# ---------------------------------------------------------------------------


def test_say_stream_logs_warning_on_repo_failure(
    fake_settings_enabled, caplog: pytest.LogCaptureFixture,
):
    """fetch_* raises → log warning + stream proceeds (no exception escapes)."""

    async def fake_stream(req):
        yield {"text": "<emotion=happy>好</emotion>", "finish_reason": None}
        yield {"text": "<end>", "finish_reason": "stop"}

    repo = AsyncMock()
    repo.fetch_player_distribution.side_effect = ConnectionError("PG down")
    repo.fetch_global_distribution.side_effect = ConnectionError("PG down")

    dispatcher = _build_dispatcher_with_repo(repo, fake_settings_enabled)
    dispatcher.llm_client.stream = fake_stream

    async def run():
        async for _ in dispatcher.say_stream(
            npc_id="npc_wang_boss_001",
            player_input="点菜",
            npc_context=[],
            session_id="sess-4",
            trace_id="tr-4",
            player_id="player-4",
        ):
            pass

    with caplog.at_level(logging.WARNING, logger="agent_os.dispatcher"):
        asyncio.run(run())

    # Warning was logged for the fetch failure (at least once)
    warnings = [r for r in caplog.records if r.levelno == logging.WARNING]
    assert any("emotion aggregate" in r.message for r in warnings), (
        f"expected warning mentioning 'emotion aggregate', got: {[r.message for r in warnings]}"
    )
    # Stream completed normally (publish_done called)
    dispatcher.publisher.publish_done.assert_called_once()


# ---------------------------------------------------------------------------
# Test 5: say_stream passes distributions to get_npc_stream_prompt
# ---------------------------------------------------------------------------


def test_say_stream_passes_distributions_to_prompt(fake_repo, fake_settings_enabled):
    """Fetched distributions are forwarded to get_npc_stream_prompt as kwargs."""

    async def fake_stream(req):
        yield {"text": "<emotion=happy>好</emotion>", "finish_reason": None}
        yield {"text": "<end>", "finish_reason": "stop"}

    recent = EmotionDistribution(
        weights={"happy": 0.6, "neutral": 0.4},
        raw_counts={"happy": 3, "neutral": 2},
        total_rows=5,
    )
    global_d = EmotionDistribution(
        weights={"sad": 0.7, "neutral": 0.3},
        raw_counts={"sad": 7, "neutral": 3},
        total_rows=10,
    )
    fake_repo.fetch_player_distribution.return_value = recent
    fake_repo.fetch_global_distribution.return_value = global_d

    dispatcher = _build_dispatcher_with_repo(fake_repo, fake_settings_enabled)
    dispatcher.llm_client.stream = fake_stream

    captured = {}

    def fake_prompt(npc_id, player_input, npc_context, **kwargs):
        captured.update(kwargs)
        return "<stub-prompt>"

    async def run():
        with patch("agent_os.dispatcher.get_npc_stream_prompt", fake_prompt):
            async for _ in dispatcher.say_stream(
                npc_id="npc_wang_boss_001",
                player_input="点菜",
                npc_context=[],
                session_id="sess-5",
                trace_id="tr-5",
                player_id="player-5",
            ):
                pass

    asyncio.run(run())

    # Both distributions forwarded as kwargs
    assert captured.get("recent_distribution") is recent
    assert captured.get("global_distribution") is global_d


# ---------------------------------------------------------------------------
# Test 6: app lifespan creates emotion_repo when PG_DSN is set
# ---------------------------------------------------------------------------


def test_app_lifespan_creates_emotion_repo_when_pg_dsn_set(
    monkeypatch, templates_dir: Path,
):
    """When PG_DSN env var is set, EmotionRepository is instantiated on app.state."""
    monkeypatch.setenv("HTTP_PORT", "8084")
    monkeypatch.setenv("REDIS_URL", "redis://127.0.0.1:6379/0")
    monkeypatch.setenv("NPC_TEMPLATES_DIR", str(templates_dir))
    monkeypatch.setenv("SAY_TICK_SECONDS", "0.05")
    monkeypatch.setenv("PG_DSN", "postgresql://test:test@localhost:5432/test")

    # Mock asyncpg.create_pool so no real connection needed
    mock_pool = AsyncMock()
    mock_pool.close = AsyncMock()

    async def fake_create_pool(*args, **kwargs):
        return mock_pool

    monkeypatch.setattr("asyncpg.create_pool", fake_create_pool)

    # Reload + build app
    import importlib

    from agent_os import app as app_module

    importlib.reload(app_module)
    from agent_os.app import create_app

    app = create_app()
    with TestClient(app):
        # During lifespan, emotion_repo is on app.state
        assert app.state.emotion_repo is not None
        assert app.state.emotion_settings is not None
        assert app.state.emotion_settings.inject_enabled is True
        # stream_dispatcher (2.0) wired with the repo
        assert app.state.stream_dispatcher.emotion_repo is app.state.emotion_repo
        assert app.state.stream_dispatcher.memory_writer is not None


# ---------------------------------------------------------------------------
# Test 7: app lifespan graceful when PG_DSN unset (no repo created)
# ---------------------------------------------------------------------------


def test_app_lifespan_no_repo_when_pg_dsn_unset(monkeypatch, templates_dir: Path):
    """Without PG_DSN, emotion_repo is None (graceful degradation)."""
    monkeypatch.setenv("HTTP_PORT", "8084")
    monkeypatch.setenv("REDIS_URL", "redis://127.0.0.1:6379/0")
    monkeypatch.setenv("NPC_TEMPLATES_DIR", str(templates_dir))
    monkeypatch.setenv("SAY_TICK_SECONDS", "0.05")
    monkeypatch.delenv("PG_DSN", raising=False)

    import importlib

    from agent_os import app as app_module

    importlib.reload(app_module)
    from agent_os.app import create_app

    app = create_app()
    with TestClient(app):
        # No repo when no PG_DSN
        assert app.state.emotion_repo is None
        # stream_dispatcher (2.0) created but no repo wired
        assert app.state.stream_dispatcher is not None
        assert app.state.stream_dispatcher.emotion_repo is None


# ---------------------------------------------------------------------------
# Test 8: app lifespan closes pool on shutdown
# ---------------------------------------------------------------------------


def test_app_lifespan_closes_pool_on_shutdown(monkeypatch, templates_dir: Path):
    """Pool is closed when lifespan shuts down."""
    monkeypatch.setenv("HTTP_PORT", "8084")
    monkeypatch.setenv("REDIS_URL", "redis://127.0.0.1:6379/0")
    monkeypatch.setenv("NPC_TEMPLATES_DIR", str(templates_dir))
    monkeypatch.setenv("SAY_TICK_SECONDS", "0.05")
    monkeypatch.setenv("PG_DSN", "postgresql://test:test@localhost:5432/test")

    mock_pool = AsyncMock()
    mock_pool.close = AsyncMock()

    async def fake_create_pool(*args, **kwargs):
        return mock_pool

    monkeypatch.setattr("agent_os.app.asyncpg.create_pool", fake_create_pool)

    import importlib

    from agent_os import app as app_module

    importlib.reload(app_module)
    from agent_os.app import create_app

    app = create_app()
    with TestClient(app):
        pass  # forces lifespan startup + shutdown

    mock_pool.close.assert_awaited_once()


# ---------------------------------------------------------------------------
# Test 9: say_stream logs aggregation latency at INFO (spec §9 #5)
# ---------------------------------------------------------------------------


def test_say_stream_logs_aggregation_latency(
    fake_repo, fake_settings_enabled, caplog: pytest.LogCaptureFixture,
):
    """When inject_enabled=True, one INFO log per say_stream call records
    npc_id, latency_ms, recent_rows, global_rows, repo class name."""
    import logging

    async def fake_stream(req):
        yield {"text": "<emotion=happy>好</emotion>", "finish_reason": None}
        yield {"text": "<end>", "finish_reason": "stop"}

    dispatcher = _build_dispatcher_with_repo(fake_repo, fake_settings_enabled)
    dispatcher.llm_client.stream = fake_stream

    async def run():
        async for _ in dispatcher.say_stream(
            npc_id="npc_wang_boss_001",
            player_input="点菜",
            npc_context=[],
            session_id="sess-latency",
            trace_id="tr-latency",
            player_id="player-latency",
        ):
            pass

    with caplog.at_level(logging.INFO, logger="agent_os.dispatcher"):
        asyncio.run(run())

    # Find the aggregation latency log
    matches = [r for r in caplog.records if "emotion aggregate" in r.message]
    assert matches, (
        f"expected at least one INFO log mentioning 'emotion aggregate', "
        f"got: {[r.message for r in caplog.records]}"
    )
    msg = matches[0].message
    assert matches[0].levelno == logging.INFO
    assert "latency_ms" in msg
    assert "recent_rows=" in msg
    assert "global_rows=" in msg
    assert "npc=npc_wang_boss_001" in msg
    # repo class name appears (AsyncMock → "AsyncMock")
    assert "repo=" in msg
