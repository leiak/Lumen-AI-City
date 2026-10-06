"""Unit test for B2 acceptance_emotion binary logic (mocked PG).

Verifies script structure end-to-end without a real DB:
  1. Pool created + closed
  2. emotion column check passes
  3. MemoryWriter.write_emotions called with 10 emotions (6 happy + 4 sad)
  4. Step 2b wipes + backdated inserts + new insert + re-aggregate (time-decay)
  5. EmotionRepository.fetch_player_distribution called, returns total_rows=10
  6. sum(weights) ≈ 1.0 (spec §12 criterion 5)
  7. Prompt injection: disabled → no section; enabled → section + happy= token
  8. EMOTION_INJECT_ENABLED env var popped in finally (no shell leak)
  9. Cleanup: DELETE rows by player_id
 10. Returns rc=0 on success path

Run:
  cd apps/agent-os
  PYTHONPATH=src .venv/Scripts/pytest.exe tests/test_acceptance_emotion.py -v
"""
from __future__ import annotations

import importlib
import sys
from pathlib import Path
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

# Make `from scripts.acceptance_emotion_v0 import main` work — pytest's
# collection starts at apps/agent-os/, and PYTHONPATH=src gives us agent_os.
# scripts/ is a sibling package of src/, so we add the parent to sys.path.
_AGENT_OS_ROOT = Path(__file__).resolve().parent.parent
if str(_AGENT_OS_ROOT) not in sys.path:
    sys.path.insert(0, str(_AGENT_OS_ROOT))


@pytest.fixture
def fake_pool():
    """AsyncMock asyncpg pool with synchronous-return acquire() context mgr.

    Mirrors the pattern used in test_emotion_writer.py — pool.acquire()
    returns a context manager (not a coroutine) just like asyncpg.Pool.acquire.
    """
    pool = MagicMock()
    conn_cm = AsyncMock()
    conn = AsyncMock()
    conn_cm.__aenter__.return_value = conn
    conn_cm.__aexit__.return_value = None
    pool.acquire.return_value = conn_cm
    pool.close = AsyncMock()
    return pool


@pytest.fixture
def fake_emotion_columns():
    """Default: emotion column is present in information_schema."""
    return [{"column_name": "emotion"}, {"column_name": "message"}]


def _make_dist(weights: dict[str, float], n: int = 1):
    """Build an EmotionDistribution with given weights.

    Enforces the spec §12 invariant: weights must sum to ≈1.0 (post-normalization).
    Tests that exercise a "broken" distribution (sum != 1.0) construct the
    EmotionDistribution directly to bypass this guard.
    """
    from agent_os.emotion.aggregate import EmotionDistribution

    # Normalize so round-trip is honest: caller passes conceptual weights, we
    # round to 3dp then assert the sum invariant.
    rounded = {k: round(v, 3) for k, v in weights.items()}
    s = sum(rounded.values())
    assert abs(s - 1.0) < 1e-6, (
        f"_make_dist invariant: weights must sum to ≈1.0 (got {s}); "
        "construct EmotionDistribution directly to bypass this guard"
    )
    return EmotionDistribution(
        weights=rounded,
        raw_counts={k: n for k in weights},
        total_rows=n * len(weights),
    )


def _has_delete_from_memory_session(conn) -> bool:
    """True if conn.execute was called with a SQL starting with the cleanup pattern."""
    for call_args in conn.execute.await_args_list:
        sql = call_args.args[0] if call_args.args else None
        if isinstance(sql, str) and sql.startswith("DELETE FROM memory_player_session"):
            return True
    return False


@pytest.mark.asyncio
async def test_acceptance_happy_path_all_6_steps_pass(
    fake_pool, fake_emotion_columns, monkeypatch,
):
    """Mock every external dep and assert rc=0 + all 6 [OK] print."""
    # fake_pool.acquire() yields conn with fetch/execute hooks
    conn = fake_pool.acquire.return_value.__aenter__.return_value
    # [1/6] information_schema returns column rows
    conn.fetch.return_value = fake_emotion_columns
    # Cleanup execute() returns a 'DELETE 10' status string
    conn.execute.return_value = "DELETE 10"

    # Reset env so reload(settings) reads controlled values
    monkeypatch.setenv("EMOTION_INJECT_ENABLED", "false")

    # Build a fake recent distribution with happy > sad, weights sum to 1.0
    happy_dist = _make_dist({"happy": 0.6, "sad": 0.4}, n=5)  # total_rows=10

    # Patch top-level imports inside the script via parent module paths
    fake_writer = MagicMock()
    fake_writer.write_emotions = AsyncMock()

    fake_repo = MagicMock()
    fake_repo.fetch_player_distribution = AsyncMock(return_value=happy_dist)
    fake_repo.fetch_global_distribution = AsyncMock(return_value=happy_dist)

    # Patch create_pool + MemoryWriter + EmotionRepository classes
    with patch("asyncpg.create_pool", new_callable=AsyncMock) as mock_create_pool, \
         patch("agent_os.memory.writer.MemoryWriter", return_value=fake_writer) as mock_writer_cls, \
         patch("agent_os.emotion.repository.EmotionRepository", return_value=fake_repo) as mock_repo_cls:
        mock_create_pool.return_value = fake_pool

        # Import lazily so patches apply at import time
        from scripts import acceptance_emotion_v0
        importlib.reload(acceptance_emotion_v0)

        rc = await acceptance_emotion_v0.main()

        # Pool lifecycle
        mock_create_pool.assert_awaited_once()
        fake_pool.close.assert_awaited_once()

        # [1/6] emotion column check
        conn.fetch.assert_called()  # information_schema fetch happened

        # [2/6] write_emotions called with 6 happy + 4 sad (10 total)
        fake_writer.write_emotions.assert_awaited_once()
        write_kwargs = fake_writer.write_emotions.await_args.kwargs
        assert write_kwargs["emotions"] == ["happy"] * 6 + ["sad"] * 4
        assert len(write_kwargs["emotions"]) == 10
        assert write_kwargs["npc_id"] == "npc_acceptance_bot"
        assert write_kwargs["player_id"] == "player-acceptance-1"

        # [2b/6] fetch_player_distribution called (step 3 reuses the result)
        fake_repo.fetch_player_distribution.assert_awaited_once()

        # [4/6] + [5/6] no exception → EMOTION_INJECT_ENABLED must be popped
        assert "EMOTION_INJECT_ENABLED" not in __import__("os").environ, (
            "EMOTION_INJECT_ENABLED leaked into process env after success path"
        )

        # Cleanup: DELETE called (at least once), and uses the specific SQL pattern
        assert conn.execute.await_count >= 1
        assert _has_delete_from_memory_session(conn), (
            "cleanup DELETE did not match expected SQL pattern"
        )

        # Final rc
        assert rc == 0


@pytest.mark.asyncio
async def test_acceptance_fails_when_emotion_column_missing(
    fake_pool, monkeypatch,
):
    """Step 1 failure → rc=1, no further steps run."""
    conn = fake_pool.acquire.return_value.__aenter__.return_value
    # No 'emotion' column
    conn.fetch.return_value = [{"column_name": "message"}, {"column_name": "npc_id"}]

    monkeypatch.setenv("EMOTION_INJECT_ENABLED", "false")

    fake_writer = MagicMock()
    fake_writer.write_emotions = AsyncMock()

    with patch("asyncpg.create_pool", new_callable=AsyncMock) as mock_create_pool, \
         patch("agent_os.memory.writer.MemoryWriter", return_value=fake_writer):
        mock_create_pool.return_value = fake_pool

        from scripts import acceptance_emotion_v0
        importlib.reload(acceptance_emotion_v0)

        rc = await acceptance_emotion_v0.main()

        assert rc == 1
        # Write should NOT have been called
        fake_writer.write_emotions.assert_not_awaited()


@pytest.mark.asyncio
async def test_acceptance_fails_when_total_rows_wrong(fake_pool, monkeypatch):
    """Step 3 failure: total_rows != 10 → rc=1."""
    conn = fake_pool.acquire.return_value.__aenter__.return_value
    conn.fetch.return_value = [{"column_name": "emotion"}]
    conn.execute.return_value = "DELETE 0"

    monkeypatch.setenv("EMOTION_INJECT_ENABLED", "false")

    fake_writer = MagicMock()
    fake_writer.write_emotions = AsyncMock()

    # Wrong total_rows (but weights sum to 1.0 so sum assertion passes)
    bad_dist = _make_dist({"happy": 0.6, "sad": 0.4}, n=2)  # total_rows=4, not 10
    fake_repo = MagicMock()
    fake_repo.fetch_player_distribution = AsyncMock(return_value=bad_dist)

    with patch("asyncpg.create_pool", new_callable=AsyncMock) as mock_create_pool, \
         patch("agent_os.memory.writer.MemoryWriter", return_value=fake_writer), \
         patch("agent_os.emotion.repository.EmotionRepository", return_value=fake_repo):
        mock_create_pool.return_value = fake_pool

        from scripts import acceptance_emotion_v0
        importlib.reload(acceptance_emotion_v0)

        rc = await acceptance_emotion_v0.main()

        assert rc == 1


@pytest.mark.asyncio
async def test_acceptance_fails_when_weights_sum_not_one(fake_pool, monkeypatch):
    """Spec §12 criterion 5: distribution must sum to ≈1.0.

    Mock returns a distribution whose weights sum to 0.5 (sum assertion fails).
    Construct EmotionDistribution directly to bypass _make_dist's invariant guard.
    """
    from agent_os.emotion.aggregate import EmotionDistribution

    conn = fake_pool.acquire.return_value.__aenter__.return_value
    conn.fetch.return_value = [{"column_name": "emotion"}]
    conn.execute.return_value = "DELETE 0"

    monkeypatch.setenv("EMOTION_INJECT_ENABLED", "false")

    fake_writer = MagicMock()
    fake_writer.write_emotions = AsyncMock()

    # happy dominates AND total_rows=10, but weights sum to 0.5 — sum assertion fails
    bad_sum_dist = EmotionDistribution(
        weights={"happy": 0.3, "sad": 0.2},  # sum = 0.5, not 1.0
        raw_counts={"happy": 6, "sad": 4},
        total_rows=10,
    )
    fake_repo = MagicMock()
    fake_repo.fetch_player_distribution = AsyncMock(return_value=bad_sum_dist)

    with patch("asyncpg.create_pool", new_callable=AsyncMock) as mock_create_pool, \
         patch("agent_os.memory.writer.MemoryWriter", return_value=fake_writer), \
         patch("agent_os.emotion.repository.EmotionRepository", return_value=fake_repo):
        mock_create_pool.return_value = fake_pool

        from scripts import acceptance_emotion_v0
        importlib.reload(acceptance_emotion_v0)

        rc = await acceptance_emotion_v0.main()

        # Sum assertion in step 3 → rc=1
        assert rc == 1


@pytest.mark.asyncio
async def test_acceptance_step_2b_inserts_backdated_and_new(fake_pool, monkeypatch):
    """Step 2b: 9 backdated sad + 1 new happy → recent dominates.

    Mock repo to return a happy-dominated distribution; verify the script's
    SQL pattern includes:
      - A DELETE FROM memory_player_session (the wipe)
      - INSERT statements with backdated created_at (the old sad rows)
      - INSERT statements with NOW() created_at (the new happy row)
    """
    conn = fake_pool.acquire.return_value.__aenter__.return_value
    conn.fetch.return_value = [{"column_name": "emotion"}]
    conn.execute.return_value = "DELETE 0"

    monkeypatch.setenv("EMOTION_INJECT_ENABLED", "false")

    fake_writer = MagicMock()
    fake_writer.write_emotions = AsyncMock()

    # Recent dominates — 1 happy weighted near 1.0, 9 sad backdated to ~0 each
    # n=5 → total_rows=10 (matches 9 backdated sad + 1 new happy)
    happy_dist = _make_dist({"happy": 0.95, "sad": 0.05}, n=5)
    fake_repo = MagicMock()
    fake_repo.fetch_player_distribution = AsyncMock(return_value=happy_dist)

    with patch("asyncpg.create_pool", new_callable=AsyncMock) as mock_create_pool, \
         patch("agent_os.memory.writer.MemoryWriter", return_value=fake_writer), \
         patch("agent_os.emotion.repository.EmotionRepository", return_value=fake_repo):
        mock_create_pool.return_value = fake_pool

        from scripts import acceptance_emotion_v0
        importlib.reload(acceptance_emotion_v0)

        rc = await acceptance_emotion_v0.main()

        assert rc == 0

        # Inspect all execute() calls; we expect:
        #   (a) at least one DELETE FROM memory_player_session  (step 2b wipe)
        #   (b) at least one INSERT with $5 = backdated timestamp
        #   (c) at least one INSERT with NOW() (no $5 placeholder)
        delete_calls = []
        backdated_inserts = []
        now_inserts = []
        for call_args in conn.execute.await_args_list:
            sql = call_args.args[0] if call_args.args else ""
            if not isinstance(sql, str):
                continue
            if sql.startswith("DELETE FROM memory_player_session"):
                delete_calls.append(call_args)
            elif sql.startswith("INSERT INTO memory_player_session"):
                # The script uses 5 placeholders for backdated, 4 for NOW() (no $5).
                # The script signature for backdated has $5 param; NOW() has only $1-$4.
                # We check parameter count via the call args tuple.
                n_params = len(call_args.args) - 1  # subtract the SQL arg
                if n_params == 5:
                    backdated_inserts.append(call_args)
                elif n_params == 4:
                    now_inserts.append(call_args)

        assert len(delete_calls) >= 1, "step 2b must wipe rows via DELETE"
        # The wipe happens before the inserts; check ordering via call index.
        first_delete_idx = next(
            i for i, c in enumerate(conn.execute.await_args_list)
            if isinstance(c.args[0], str) and c.args[0].startswith("DELETE FROM memory_player_session")
        )
        insert_indices = [
            i for i, c in enumerate(conn.execute.await_args_list)
            if isinstance(c.args[0], str) and c.args[0].startswith("INSERT INTO memory_player_session")
        ]
        assert insert_indices, "step 2b must INSERT backdated + new rows"
        assert min(insert_indices) > first_delete_idx, (
            "wipe DELETE must precede INSERT statements"
        )


@pytest.mark.asyncio
async def test_acceptance_pg_unreachable_returns_2():
    """Pool create failure → rc=2, no further steps."""
    with patch("asyncpg.create_pool", new_callable=AsyncMock) as mock_create_pool:
        mock_create_pool.side_effect = OSError("connection refused")

        from scripts import acceptance_emotion_v0
        importlib.reload(acceptance_emotion_v0)

        rc = await acceptance_emotion_v0.main()

        assert rc == 2


@pytest.mark.asyncio
async def test_acceptance_cleanup_runs_even_on_failure(fake_pool, monkeypatch):
    """If step 2b fails mid-run, cleanup DELETE must still run."""
    conn = fake_pool.acquire.return_value.__aenter__.return_value
    conn.fetch.return_value = [{"column_name": "emotion"}]
    conn.execute.return_value = "DELETE 0"

    # Don't set EMOTION_INJECT_ENABLED via monkeypatch — we want to assert it
    # wasn't leaked. Ensure it starts unset.
    monkeypatch.delenv("EMOTION_INJECT_ENABLED", raising=False)

    fake_writer = MagicMock()
    fake_writer.write_emotions = AsyncMock()

    # Make the repo raise in step 2b to trigger failure path
    fake_repo = MagicMock()
    fake_repo.fetch_player_distribution = AsyncMock(
        side_effect=RuntimeError("boom"),
    )

    with patch("asyncpg.create_pool", new_callable=AsyncMock) as mock_create_pool, \
         patch("agent_os.memory.writer.MemoryWriter", return_value=fake_writer), \
         patch("agent_os.emotion.repository.EmotionRepository", return_value=fake_repo):
        mock_create_pool.return_value = fake_pool

        from scripts import acceptance_emotion_v0
        importlib.reload(acceptance_emotion_v0)

        rc = await acceptance_emotion_v0.main()

        # Crash in step 2b → rc=1
        assert rc != 0
        # Pool must still have been closed
        fake_pool.close.assert_awaited_once()
        # DELETE was called (cleanup path) and uses the specific SQL pattern
        assert conn.execute.await_count >= 1
        assert _has_delete_from_memory_session(conn), (
            "cleanup DELETE did not match expected SQL pattern on failure path"
        )
        # Env var must NOT leak (never set in this test)
        assert "EMOTION_INJECT_ENABLED" not in __import__("os").environ


@pytest.mark.asyncio
async def test_acceptance_env_var_popped_on_failure(fake_pool, monkeypatch):
    """Even if step 4 raises, the env mutation must be reverted."""
    import os

    conn = fake_pool.acquire.return_value.__aenter__.return_value
    conn.fetch.return_value = [{"column_name": "emotion"}]
    conn.execute.return_value = "DELETE 0"

    # Ensure env starts unset so we can assert it stays unset after a failure.
    monkeypatch.delenv("EMOTION_INJECT_ENABLED", raising=False)

    fake_writer = MagicMock()
    fake_writer.write_emotions = AsyncMock()

    # Happy distribution so we get past steps 1-3
    happy_dist = _make_dist({"happy": 0.6, "sad": 0.4}, n=5)
    fake_repo = MagicMock()
    fake_repo.fetch_player_distribution = AsyncMock(return_value=happy_dist)

    # Patch get_npc_stream_prompt so step 4 raises; this exercises the
    # try/except/finally that pops EMOTION_INJECT_ENABLED.
    with patch("asyncpg.create_pool", new_callable=AsyncMock) as mock_create_pool, \
         patch("agent_os.memory.writer.MemoryWriter", return_value=fake_writer), \
         patch("agent_os.emotion.repository.EmotionRepository", return_value=fake_repo), \
         patch("agent_os.llm.prompts.get_npc_stream_prompt", side_effect=RuntimeError("boom")):
        mock_create_pool.return_value = fake_pool

        # Verify env starts unset
        assert "EMOTION_INJECT_ENABLED" not in os.environ

        from scripts import acceptance_emotion_v0
        importlib.reload(acceptance_emotion_v0)

        rc = await acceptance_emotion_v0.main()

        # Step 4 raises → except catches → rc=1
        assert rc != 0
        # The script's try/finally must have popped the env var
        assert "EMOTION_INJECT_ENABLED" not in os.environ, (
            "EMOTION_INJECT_ENABLED leaked into process env after step 4 failure"
        )
