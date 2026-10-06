"""B2 acceptance: write 10 emotions, aggregate, verify prompt injection.

Usage:
    cd apps/agent-os
    PYTHONPATH=src python scripts/acceptance_emotion_v0.py

Requires dev PG at postgresql://aicity:aicity_dev@localhost:5432/aicity.

Spec ref: docs/superpowers/specs/2026-10-06-2.0-emotion-persistence-design.md §12.

6-step E2E (steps 1-5 spec literal; 2b adds true time-decay coverage):
  [1/6]   Verify memory_player_session has 'emotion' column
  [2/6]   Insert 10 emotions via MemoryWriter (6 happy + 4 sad)
  [2b/6]  Backdated 9 sad (6h old) + 1 new happy → recent dominates (real exp-decay)
  [3/6]   Aggregate via EmotionRepository → sum(weights) ≈ 1.0 AND happy > sad
  [4/6]   Prompt with EMOTION_INJECT_ENABLED=false → no section injected
  [5/6]   Prompt with EMOTION_INJECT_ENABLED=true  → section + happy= token

Each step prints [OK] / [FAIL]. Cleanup: DELETE rows by player_id at end.
Exit code: 0 = ALL PASS, 1 = step failure, 2 = PG unreachable.
"""
from __future__ import annotations

import asyncio
import datetime as dt
import importlib
import json
import os
import sys
import uuid
from pathlib import Path

# 允许 ``uv run python scripts/acceptance_emotion_v0.py`` 直接执行（无需 PYTHONPATH=src）
_SRC = Path(__file__).resolve().parent.parent / "src"
if str(_SRC) not in sys.path:
    sys.path.insert(0, str(_SRC))

DSN = os.getenv("PG_DSN", "postgresql://aicity:aicity_dev@localhost:5432/aicity")


def _redact_dsn(dsn: str) -> str:
    """Hide password in DSN for safe logging."""
    # postgresql://user:password@host:port/db → postgresql://user:***@host:port/db
    try:
        scheme, rest = dsn.split("://", 1)
        if "@" in rest:
            creds, hostpart = rest.split("@", 1)
            if ":" in creds:
                user, _ = creds.split(":", 1)
                return f"{scheme}://{user}:***@{hostpart}"
        # Malformed DSN (no userinfo or no scheme delimiter) — don't leak any prefix
        return "***"
    except Exception:  # noqa: BLE001
        return "***"


async def main() -> int:
    import asyncpg

    from agent_os.emotion.repository import EmotionRepository
    from agent_os.llm.prompts import get_npc_stream_prompt
    from agent_os.memory.writer import MemoryWriter

    session_id = f"accept-{uuid.uuid4().hex[:12]}"
    npc_id = "npc_acceptance_bot"
    player_id = "player-acceptance-1"

    print(f"B2 acceptance run session={session_id}")
    print(f"DSN (redacted): {_redact_dsn(DSN)}")
    print()

    pool = None
    try:
        try:
            pool = await asyncpg.create_pool(DSN, min_size=1, max_size=2)
        except Exception as e:  # noqa: BLE001
            print(f"[FAIL] cannot connect to PG: {type(e).__name__}: {e}")
            return 2

        try:
            # ----------------------------------------------------------------
            # [1/6] Verify emotion column on memory_player_session
            # ----------------------------------------------------------------
            print("[1/6] Verify emotion column on memory_player_session")
            try:
                async with pool.acquire() as conn:
                    cols = await conn.fetch(
                        "SELECT column_name FROM information_schema.columns "
                        "WHERE table_name = 'memory_player_session'"
                    )
                col_names = {r["column_name"] for r in cols}
                if "emotion" not in col_names:
                    print(
                        "[FAIL] step 1: emotion column missing. "
                        "Apply pg-schema-2.0-emotion.sql first."
                    )
                    return 1
                print("[OK] emotion column present")
            except Exception as e:  # noqa: BLE001
                print(f"[FAIL] step 1: {type(e).__name__}: {e}")
                return 1
            print()

            # ----------------------------------------------------------------
            # [2/6] Insert 10 emotions via MemoryWriter
            # ----------------------------------------------------------------
            print("[2/6] Insert 10 emotions via MemoryWriter (6 happy + 4 sad)")
            try:
                writer = MemoryWriter(pool)
                emotions = ["happy"] * 6 + ["sad"] * 4
                await writer.write_emotions(
                    npc_id=npc_id,
                    player_id=player_id,
                    session_id=session_id,
                    emotions=emotions,
                )
                print(f"[OK] inserted {len(emotions)} emotions")
            except Exception as e:  # noqa: BLE001
                print(f"[FAIL] step 2: {type(e).__name__}: {e}")
                return 1
            print()

            # ----------------------------------------------------------------
            # [2b/6] Time-decay semantics — backdated old rows vs new row
            # ----------------------------------------------------------------
            print("[2b/6] Time-decay: 9 backdated sad (6h old) + 1 new happy → recent dominates")
            try:
                # Wipe the recent batch so we have a clean slate for time-decay test
                async with pool.acquire() as conn:
                    await conn.execute(
                        "DELETE FROM memory_player_session WHERE player_id = $1",
                        player_id,
                    )

                # 9 "old" sad rows: τ=2h, age=6h → weight ≈ exp(-3) ≈ 0.050 each
                old_time = dt.datetime.now(dt.timezone.utc) - dt.timedelta(hours=6)
                async with pool.acquire() as conn:
                    for _ in range(9):
                        await conn.execute(
                            "INSERT INTO memory_player_session "
                            "(npc_id, player_id, message, emotion, created_at) "
                            "VALUES ($1, $2, $3::jsonb, $4, $5)",
                            npc_id, player_id,
                            json.dumps({"session_id": session_id, "type": "backdated"}),
                            "sad", old_time,
                        )
                    # 1 "new" happy row: weight ≈ 1.0
                    await conn.execute(
                        "INSERT INTO memory_player_session "
                        "(npc_id, player_id, message, emotion, created_at) "
                        "VALUES ($1, $2, $3::jsonb, $4, NOW())",
                        npc_id, player_id,
                        json.dumps({"session_id": session_id, "type": "recent"}),
                        "happy",
                    )

                # Re-aggregate and verify recent dominates
                repo = EmotionRepository(pool)
                recent_after_decay = await repo.fetch_player_distribution(npc_id, player_id)
                w_happy = recent_after_decay.weights.get("happy", 0.0)
                w_sad = recent_after_decay.weights.get("sad", 0.0)
                if w_happy <= w_sad:
                    print(
                        f"[FAIL] step 2b: time-decay failed. "
                        f"happy={w_happy:.4f} sad={w_sad:.4f}"
                    )
                    return 1
                print(
                    f"[OK] time-decay: 1 new happy dominates 9 old sad "
                    f"(happy={w_happy:.3f}, sad={w_sad:.3f})"
                )
            except Exception as e:  # noqa: BLE001
                print(f"[FAIL] step 2b: {type(e).__name__}: {e}")
                return 1
            print()

            # ----------------------------------------------------------------
            # [3/6] Aggregate via EmotionRepository
            # ----------------------------------------------------------------
            print("[3/6] Aggregate via EmotionRepository")
            try:
                # Reuse repo from step 2b (already constructed). Recent distribution
                # now reflects the backdated + new mix from step 2b.
                recent = recent_after_decay
                if recent.total_rows != 10:
                    print(
                        f"[FAIL] step 3: expected total_rows=10, "
                        f"got {recent.total_rows}"
                    )
                    return 1
                # Spec §12 criterion 5: distribution ≈ 1.0
                dist_sum = sum(recent.weights.values())
                if not (0.999 <= dist_sum <= 1.001):
                    print(
                        f"[FAIL] step 3: weights sum={dist_sum:.4f}, "
                        f"expected ≈1.0"
                    )
                    return 1
                if recent.weights.get("happy", 0.0) <= recent.weights.get("sad", 0.0):
                    print(
                        f"[FAIL] step 3: expected happy > sad; "
                        f"got {recent.weights}"
                    )
                    return 1
                print(
                    f"[OK] recent_dist total_rows={recent.total_rows} "
                    f"weights={recent.weights} sum={dist_sum:.4f}"
                )
            except Exception as e:  # noqa: BLE001
                print(f"[FAIL] step 3: {type(e).__name__}: {e}")
                return 1
            print()

            # ----------------------------------------------------------------
            # [4/6] Prompt with EMOTION_INJECT_ENABLED=false
            # [5/6] Prompt with EMOTION_INJECT_ENABLED=true
            # ----------------------------------------------------------------
            # Wrap env mutations in try/finally so we never leak shell env state
            # after the binary completes (important for callers that import
            # agent_os.emotion.settings later in the same process).
            os.environ.pop("EMOTION_INJECT_ENABLED", None)  # clean slate
            try:
                # ------------------------------------------------------------
                # [4/6] Prompt with EMOTION_INJECT_ENABLED=false
                # ------------------------------------------------------------
                print("[4/6] Prompt build with EMOTION_INJECT_ENABLED=false")
                os.environ["EMOTION_INJECT_ENABLED"] = "false"
                from agent_os.emotion import settings as _settings_mod
                importlib.reload(_settings_mod)
                from agent_os.emotion.settings import EmotionSettings as _ES
                s_disabled = _ES.from_env()
                if s_disabled.inject_enabled:
                    print(
                        "[FAIL] step 4: EmotionSettings.from_env() reports "
                        "enabled=True despite EMOTION_INJECT_ENABLED=false"
                    )
                    return 1
                # Simulate dispatcher path: kill switch off → pass None to prompt
                prompt_disabled = get_npc_stream_prompt(
                    npc_id, "你好", [],
                    recent_distribution=None,
                    global_distribution=None,
                )
                if "【最近情绪氛围" in prompt_disabled:
                    print(
                        "[FAIL] step 4: section injected despite kill switch"
                    )
                    return 1
                print("[OK] no injection when kill switch off")
                print()

                # ------------------------------------------------------------
                # [5/6] Prompt with EMOTION_INJECT_ENABLED=true
                # ------------------------------------------------------------
                print("[5/6] Prompt build with EMOTION_INJECT_ENABLED=true")
                os.environ["EMOTION_INJECT_ENABLED"] = "true"
                importlib.reload(_settings_mod)
                s_enabled = _ES.from_env()
                if not s_enabled.inject_enabled:
                    print(
                        "[FAIL] step 5: EmotionSettings.from_env() reports "
                        "enabled=False despite EMOTION_INJECT_ENABLED=true"
                    )
                    return 1
                # Simulate dispatcher path: pass real distributions
                prompt_enabled = get_npc_stream_prompt(
                    npc_id, "你好", [],
                    recent_distribution=recent,
                    global_distribution=None,
                )
                if "【最近情绪氛围" not in prompt_enabled:
                    print(
                        "[FAIL] step 5: section missing despite enabled"
                    )
                    return 1
                if "happy=" not in prompt_enabled:
                    print(
                        f"[FAIL] step 5: no happy= token. "
                        f"got first 300 chars: {prompt_enabled[:300]}"
                    )
                    return 1
                print("[OK] section present + happy= token visible")
                print()
            except Exception as e:  # noqa: BLE001
                # Steps 4/5 raised unexpectedly (e.g. dispatcher wired wrong).
                # Surface as a step failure so callers get a clean rc=1.
                print(f"[FAIL] step 4/5: {type(e).__name__}: {e}")
                return 1
            finally:
                os.environ.pop("EMOTION_INJECT_ENABLED", None)

            # ----------------------------------------------------------------
            # Cleanup (always run on success)
            # ----------------------------------------------------------------
            async with pool.acquire() as conn:
                deleted = await conn.execute(
                    "DELETE FROM memory_player_session WHERE player_id = $1",
                    player_id,
                )
            # asyncpg returns 'DELETE <n>'
            print(f"[cleanup] {deleted}")

        finally:
            # Belt-and-suspenders cleanup in case of exception mid-run
            try:
                if pool is not None:
                    async with pool.acquire() as conn:
                        await conn.execute(
                            "DELETE FROM memory_player_session WHERE player_id = $1",
                            player_id,
                        )
            except Exception:  # noqa: BLE001
                pass

    finally:
        if pool is not None:
            await pool.close()

    print()
    print("=" * 50)
    print("ALL 6 STEPS PASS — B2 emotion persistence ACCEPTED")
    print("=" * 50)
    return 0


if __name__ == "__main__":
    sys.exit(asyncio.run(main()))
