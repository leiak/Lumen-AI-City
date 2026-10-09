from __future__ import annotations

from pathlib import Path


def migration_path() -> Path:
    return Path(__file__).parents[3] / "packages" / "proto" / "pg-schema-3.0-cross-city-gold.sql"


def test_cross_city_schema_contains_bridge_tables() -> None:
    sql = migration_path().read_text(encoding="utf-8")

    for token in (
        "CREATE TABLE IF NOT EXISTS cross_city_transfer",
        "CREATE TABLE IF NOT EXISTS bridge_position",
        "CREATE TABLE IF NOT EXISTS bridge_ledger_entry",
        "UNIQUE (direction, idempotency_key)",
        "UNIQUE (global_id, direction)",
        "idx_cct_outbound_reconcile",
        "idx_bridge_entry_transfer",
        "cross_city_out",
        "cross_city_in",
    ):
        assert token in sql, f"migration missing {token}"


def test_cross_city_schema_limits_gold_and_amount() -> None:
    sql = migration_path().read_text(encoding="utf-8")

    assert "CHECK (currency = 'gold')" in sql
    assert "CHECK (amount > 0 AND amount <= 100000)" in sql
