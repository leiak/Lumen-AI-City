#!/usr/bin/env python3
"""Collect migration, Kafka offset, and backup evidence for M4 D3.3."""

from __future__ import annotations

import argparse
import hashlib
import json
import subprocess
import sys
import time
from datetime import datetime, timezone
from pathlib import Path
from typing import Any


def utc_now() -> str:
    return datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")


def atomic_write(path: Path, payload: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    temporary = path.with_suffix(path.suffix + ".tmp")
    temporary.write_text(json.dumps(payload, ensure_ascii=False) + "\n", encoding="utf-8")
    temporary.replace(path)


def docker_exec(
    container: str,
    command: list[str],
    timeout: float = 30,
) -> tuple[int, str, str]:
    completed = subprocess.run(
        ["docker", "exec", container, *command],
        capture_output=True,
        text=True,
        timeout=timeout,
        check=False,
    )
    return completed.returncode, completed.stdout.strip(), completed.stderr.strip()


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as file_handle:
        for block in iter(lambda: file_handle.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def migration_evidence(
    manifest_path: Path,
    repo_root: Path,
    postgres_container: str,
    postgres_user: str,
    postgres_database: str,
) -> dict[str, Any]:
    query = "SELECT filename, checksum, applied_at FROM schema_migrations ORDER BY filename"
    code, output, error = docker_exec(
        postgres_container,
        ["psql", "-At", "-F", "|", "-U", postgres_user, "-d", postgres_database, "-c", query],
    )
    if code != 0:
        return {"ok": False, "error": error or "failed to query schema_migrations"}

    database_rows: dict[str, dict[str, str]] = {}
    for line in output.splitlines():
        if not line:
            continue
        try:
            filename, checksum, applied_at = line.split("|", 2)
        except ValueError:
            return {"ok": False, "error": f"invalid schema_migrations row: {line}"}
        database_rows[filename] = {"checksum": checksum, "applied_at": applied_at}

    manifest_files: list[str] = []
    if manifest_path.exists():
        for raw_line in manifest_path.read_text(encoding="utf-8").splitlines():
            filename = raw_line.strip()
            if filename and not filename.startswith("#"):
                manifest_files.append(filename)
    else:
        return {"ok": False, "error": f"migration manifest not found: {manifest_path}"}

    checks: list[dict[str, Any]] = []
    for filename in manifest_files:
        migration_path = repo_root / filename
        if not migration_path.exists():
            checks.append({"file": filename, "ok": False, "error": "file missing"})
            continue
        checksum = sha256_file(migration_path)
        row = database_rows.get(filename)
        if row is None:
            checks.append({"file": filename, "ok": False, "error": "not applied"})
        elif row["checksum"] != checksum:
            checks.append({"file": filename, "ok": False, "error": "checksum mismatch"})
        else:
            checks.append({"file": filename, "ok": True, "applied_at": row["applied_at"]})

    unexpected = sorted(set(database_rows) - set(manifest_files))
    if unexpected:
        checks.append({"ok": False, "error": "unexpected database migrations", "files": unexpected})
    return {"ok": all(item.get("ok") for item in checks), "checks": checks}


def topics_from_env(path: Path) -> list[str]:
    topics: list[str] = []
    if not path.exists():
        return topics
    for line in path.read_text(encoding="utf-8").splitlines():
        stripped = line.strip()
        if stripped and not stripped.startswith("#") and stripped != "topic|partitions|configs":
            topic = stripped.split("|", 1)[0]
            topics.append(topic)
    return topics


def kafka_offset_evidence(
    topics: list[str],
    kafka_container: str,
) -> dict[str, Any]:
    offsets: dict[str, list[dict[str, int]]] = {}
    failures: list[str] = []
    for topic in topics:
        code, output, error = docker_exec(
            kafka_container,
            [
                "kafka-run-class",
                "kafka.tools.GetOffsetShell",
                "--bootstrap-server",
                "localhost:9092",
                "--topic",
                topic,
                "--time",
                "-1",
            ],
        )
        if code != 0:
            failures.append(f"{topic}: {error or 'offset query failed'}")
            continue
        partitions: list[dict[str, int]] = []
        for line in output.splitlines():
            fields = line.strip().rsplit(":", 2)
            if len(fields) == 3 and fields[0] == topic:
                partitions.append({"partition": int(fields[1]), "offset": int(fields[2])})
        if not partitions:
            failures.append(f"{topic}: no offsets returned")
        offsets[topic] = sorted(partitions, key=lambda item: item["partition"])
    return {"ok": not failures, "offsets": offsets, "failures": failures}


def backup_evidence(backup_dir: Path, require_backup: bool) -> dict[str, Any]:
    if not backup_dir.exists():
        result: dict[str, Any] = {"ok": not require_backup, "backups_found": 0}
        if require_backup:
            result["error"] = f"backup directory not found: {backup_dir}"
        return result

    backups = sorted(
        (item for item in backup_dir.glob("*.dump") if item.is_file()),
        key=lambda item: item.stat().st_mtime,
        reverse=True,
    )
    result = {"ok": not require_backup, "backups_found": len(backups)}
    if not backups:
        if require_backup:
            result["error"] = "no backups found"
        return result

    latest = backups[0]
    checksum_path = latest.with_name(latest.name + ".sha256")
    if not checksum_path.exists():
        result.update({"ok": False, "error": f"missing checksum: {checksum_path}"})
        return result
    expected = checksum_path.read_text(encoding="utf-8").strip().split()[0]
    actual = sha256_file(latest)
    result.update(
        {
            "latest": str(latest),
            "size_bytes": latest.stat().st_size,
            "sha256": actual,
            "ok": actual == expected,
        }
    )
    if actual != expected:
        result["error"] = "latest backup checksum mismatch"
    return result


def snapshot(args: argparse.Namespace) -> dict[str, Any]:
    repo_root = Path(args.repo_root).resolve()
    manifest_path = Path(args.manifest)
    if not manifest_path.is_absolute():
        manifest_path = repo_root / manifest_path
    backup_dir = Path(args.backup_dir)
    if not backup_dir.is_absolute():
        backup_dir = repo_root / backup_dir
    topics = topics_from_env(repo_root / args.topics_env)

    migrations = migration_evidence(
        manifest_path,
        repo_root,
        args.postgres_container,
        args.postgres_user,
        args.postgres_database,
    )
    kafka = kafka_offset_evidence(topics, args.kafka_container)
    backups = backup_evidence(backup_dir, args.require_backup)
    failures = [
        name
        for name, evidence in {
            "migrations": migrations,
            "kafka": kafka,
            "backups": backups,
        }.items()
        if not evidence.get("ok")
    ]
    return {
        "updated_at": utc_now(),
        "ok": not failures,
        "failures": failures,
        "migrations": migrations,
        "kafka": kafka,
        "backups": backups,
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--repo-root", default=".")
    parser.add_argument("--postgres-container", default="aitown-postgres-1")
    parser.add_argument("--postgres-user", default="aicity")
    parser.add_argument("--postgres-database", default="aicity")
    parser.add_argument("--kafka-container", default="kafka")
    parser.add_argument("--manifest", default="db/migrations/manifest.txt")
    parser.add_argument("--topics-env", default="deploy/kafka/topics.env")
    parser.add_argument("--backup-dir", default="backups/production")
    parser.add_argument("--require-backup", action="store_true")
    parser.add_argument("--output", default="artifacts/stability/m4-integrity-state.json")
    parser.add_argument("--evidence", default="artifacts/stability/m4-integrity.jsonl")
    parser.add_argument("--interval-seconds", type=int, default=86_400)
    parser.add_argument("--once", action="store_true")
    args = parser.parse_args()

    while True:
        current = snapshot(args)
        atomic_write(Path(args.output), current)
        evidence_path = Path(args.evidence)
        evidence_path.parent.mkdir(parents=True, exist_ok=True)
        with evidence_path.open("a", encoding="utf-8") as file_handle:
            file_handle.write(json.dumps(current, ensure_ascii=False) + "\n")
        print(json.dumps(current, ensure_ascii=False), flush=True)
        if args.once:
            return 0 if current["ok"] else 1
        time.sleep(max(1, args.interval_seconds))


if __name__ == "__main__":
    sys.exit(main())
