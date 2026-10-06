"""B2 acceptance: write 10 emotions, aggregate, verify prompt injection.

Usage:
    cd apps/agent-os
    PYTHONPATH=src python scripts/acceptance_emotion_v0.py

Requires dev PG at postgresql://aicity:aicity_dev@localhost:5432/aicity.

Spec ref: docs/superpowers/specs/2026-10-06-2.0-emotion-persistence-design.md §12.

5-step E2E:
  [1/5] Verify memory_player_session has 'emotion' column
  [2/5] Insert 10 emotions via MemoryWriter (6 happy + 4 sad)
  [3/5] Aggregate via EmotionRepository → happy weight > sad weight
  [4/5] Prompt with EMOTION_INJECT_ENABLED=false → no section injected
  [5/5] Prompt with EMOTION_INJECT_ENABLED=true  → section + happy= token

Each step prints [OK] / [FAIL]. Cleanup: DELETE rows by player_id at end.
Exit code: 0 = ALL PASS, 1 = step failure, 2 = PG unreachable.
"""
from __future__ import annotations

import asyncio
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
        return f"{scheme}://{rest[:8]}..."
    except Exception:  # noqa: BLE001
        return "<redacted>"


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
            # [1/5] Verify emotion column on memory_player_session
            # ----------------------------------------------------------------
            print("[1/5] Verify emotion column on memory_player_session")
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
            # [2/5] Insert 10 emotions via MemoryWriter
            # ----------------------------------------------------------------
            print("[2/5] Insert 10 emotions via MemoryWriter (6 happy + 4 sad)")
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

            # Tiny sleep so created_at is measurable (exp decay needs time delta)
            await asyncio.sleep(0.05)

            # ----------------------------------------------------------------
            # [3/5] Aggregate via EmotionRepository
            # ----------------------------------------------------------------
            print("[3/5] Aggregate via EmotionRepository")
            try:
                repo = EmotionRepository(pool)
                recent = await repo.fetch_player_distribution(npc_id, player_id)
                if recent.total_rows != 10:
                    print(
                        f"[FAIL] step 3: expected total_rows=10, "
                        f"got {recent.total_rows}"
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
                    f"weights={recent.weights}"
                )
            except Exception as e:  # noqa: BLE001
                print(f"[FAIL] step 3: {type(e).__name__}: {e}")
                return 1
            print()

            # ----------------------------------------------------------------
            # [4/5] Prompt with EMOTION_INJECT_ENABLED=false
            # ----------------------------------------------------------------
            print("[4/5] Prompt build with EMOTION_INJECT_ENABLED=false")
            try:
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
            except Exception as e:  # noqa: BLE001
                print(f"[FAIL] step 4: {type(e).__name__}: {e}")
                return 1
            print()

            # ----------------------------------------------------------------
            # [5/5] Prompt with EMOTION_INJECT_ENABLED=true
            # ----------------------------------------------------------------
            print("[5/5] Prompt build with EMOTION_INJECT_ENABLED=true")
            try:
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
            except Exception as e:  # noqa: BLE001
                print(f"[FAIL] step 5: {type(e).__name__}: {e}")
                return 1
            print()

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
    print("ALL 5 STEPS PASS — B2 emotion persistence ACCEPTED")
    print("=" * 50)
    return 0


if __name__ == "__main__":
    sys.exit(asyncio.run(main()))
