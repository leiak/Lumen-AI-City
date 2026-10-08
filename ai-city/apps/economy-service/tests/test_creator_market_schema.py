"""Contract tests for the creator-market database migration.

The economy tests are currently mock-backed, so this keeps W1 focused on the
migration contract before services are introduced. It intentionally checks the
real PostgreSQL types needed for player foreign keys.
"""

from __future__ import annotations

import re
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]
MIGRATION = ROOT / "packages" / "proto" / "pg-schema-3.0-creator-market.sql"
SEED = ROOT / "db" / "seed" / "seed-creator-market.sql"
COMPOSE = ROOT / "docker-compose.yml"


def normalized(value: str) -> str:
    return re.sub(r"\s+", " ", value)


def test_creator_market_migration_uses_player_uuid_foreign_keys() -> None:
    sql = MIGRATION.read_text(encoding="utf-8")
    compact_sql = normalized(sql)

    assert "CREATE TABLE IF NOT EXISTS npc_template" in sql
    assert "creator_id UUID NOT NULL REFERENCES player(id)" in compact_sql
    assert "CHECK (price_gold >= 10)" in sql
    assert "CHECK (status IN ('live', 'taken_down'))" in sql

    assert "CREATE TABLE IF NOT EXISTS saga_template" in sql
    assert "npc_deps TEXT[] NOT NULL DEFAULT '{}'" in compact_sql

    assert "CREATE TABLE IF NOT EXISTS template_purchase" in sql
    assert "user_id UUID NOT NULL REFERENCES player(id)" in compact_sql
    assert "CHECK (template_kind IN ('npc', 'saga'))" in sql
    assert "idempotency_key TEXT UNIQUE" in sql

    assert "CREATE TABLE IF NOT EXISTS creator_revenue" in sql
    assert "creator_id UUID NOT NULL REFERENCES player(id)" in compact_sql
    assert "platform_cut_gold BIGINT NOT NULL DEFAULT 0" in sql


def test_role_extension_allows_creator_without_backsliding_admin() -> None:
    sql = MIGRATION.read_text(encoding="utf-8")
    compact_sql = normalized(sql)

    constraint = re.search(
        r"ADD CONSTRAINT player_role_three_roles_check "
        r"CHECK \(role IN \('player', 'creator', 'admin'\)\)",
        compact_sql,
        re.IGNORECASE,
    )
    assert constraint is not None
    assert "UPDATE player SET role = 'creator'" not in sql


def test_seed_promotes_demo_and_adds_creator() -> None:
    sql = SEED.read_text(encoding="utf-8")
    compact_sql = normalized(sql)

    assert "UPDATE player SET role = 'creator' WHERE username = 'demo'" in compact_sql
    assert "'creator_demo'" in sql
    assert "crypt(" in sql
    assert "'creator'" in sql


def test_compose_mounts_creator_market_after_economy_slots() -> None:
    compose = COMPOSE.read_text(encoding="utf-8")

    assert "pg-schema-3.0-creator-market.sql:/docker-entrypoint-initdb.d/09-creator-market.sql:ro" in compose
    assert "seed-creator-market.sql:/docker-entrypoint-initdb.d/10-seed-creator-market.sql:ro" in compose
