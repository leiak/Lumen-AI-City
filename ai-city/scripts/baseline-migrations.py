#!/usr/bin/env python3
"""Baseline schema_migrations for databases initialized by Compose bootstrap SQL."""

from __future__ import annotations

import argparse
import hashlib
import json
import subprocess
import sys
from pathlib import Path


CREATE_TABLE = """
CREATE TABLE IF NOT EXISTS schema_migrations (
    filename TEXT PRIMARY KEY,
    checksum TEXT NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
)
"""


def sql_literal(value: str) -> str:
    return "'" + value.replace("'", "''") + "'"


def psql(
    container: str,
    user: str,
    database: str,
    sql: str,
    variables: list[str] | None = None,
) -> None:
    command = ["docker", "exec", "-i", container, "psql", "-v", "ON_ERROR_STOP=1"]
    for variable in variables or []:
        command.extend(["-v", variable])
    command.extend(["-U", user, "-d", database, "-c", sql])
    completed = subprocess.run(command, text=True, capture_output=True, check=False)
    if completed.returncode != 0:
        raise RuntimeError(completed.stderr.strip() or completed.stdout.strip())


def manifest_files(path: Path) -> list[str]:
    filenames: list[str] = []
    for raw_line in path.read_text(encoding="utf-8").splitlines():
        filename = raw_line.strip()
        if filename and not filename.startswith("#"):
            filenames.append(filename)
    return filenames


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--repo-root", default=".")
    parser.add_argument("--manifest", default="db/migrations/manifest.txt")
    parser.add_argument("--postgres-container", default="aitown-postgres-1")
    parser.add_argument("--postgres-user", default="aicity")
    parser.add_argument("--postgres-database", default="aicity")
    args = parser.parse_args()

    repo_root = Path(args.repo_root).resolve()
    manifest_path = Path(args.manifest)
    if not manifest_path.is_absolute():
        manifest_path = repo_root / manifest_path
    try:
        psql(args.postgres_container, args.postgres_user, args.postgres_database, CREATE_TABLE)
        processed = 0
        for filename in manifest_files(manifest_path):
            migration_path = repo_root / filename
            if not migration_path.exists():
                print(f"migration file missing: {filename}", file=sys.stderr)
                return 1
            checksum = hashlib.sha256(migration_path.read_bytes()).hexdigest()
            psql(
                args.postgres_container,
                args.postgres_user,
                args.postgres_database,
                f"INSERT INTO schema_migrations (filename, checksum) VALUES ({sql_literal(filename)}, {sql_literal(checksum)}) ON CONFLICT DO NOTHING",
            )
            processed += 1
        print(json.dumps({"processed": processed}))
        return 0
    except (OSError, RuntimeError, ValueError) as error:
        print(f"baseline failed: {error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
