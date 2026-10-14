#!/usr/bin/env python3
"""Render a human-readable M4 stability report from the 7-day evidence log."""

from __future__ import annotations

import argparse
import json
import math
import statistics
import sys
from collections.abc import Iterable
from datetime import datetime, timedelta, timezone
from pathlib import Path
from typing import Any


MAX_ERROR_RATE = 0.001
MAX_P95_SECONDS = 2.0
MIN_UPTIME = 0.999


def utc_now() -> str:
    return datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")


def parse_timestamp(value: str) -> datetime:
    return datetime.fromisoformat(value.replace("Z", "+00:00"))


def atomic_write(path: Path, content: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    temporary = path.with_suffix(path.suffix + ".tmp")
    temporary.write_text(content, encoding="utf-8", newline="\n")
    temporary.replace(path)


def read_evidence(path: Path) -> list[dict[str, Any]]:
    records: list[dict[str, Any]] = []
    if not path.exists():
        return records
    for line in path.read_text(encoding="utf-8").splitlines():
        if not line.strip():
            continue
        try:
            item = json.loads(line)
        except json.JSONDecodeError as error:
            print(f"warning: skipped invalid evidence line: {error}", file=sys.stderr)
            continue
        if isinstance(item, dict):
            records.append(item)
    return records


def checked_results(results: Iterable[dict[str, Any]]) -> list[dict[str, Any]]:
    return [item for item in results if not str(item.get("check", "")).startswith("auth.")]


def result_stats(results: Iterable[dict[str, Any]]) -> dict[str, Any]:
    checked = checked_results(results)
    failures = [item for item in checked if not item.get("ok")]
    latencies = sorted(float(item.get("elapsed_seconds", 0)) for item in checked)
    p95 = (
        statistics.quantiles(latencies, n=100, method="inclusive")[94]
        if len(latencies) >= 20
        else max(latencies, default=0.0)
    )
    return {
        "checks": len(checked),
        "failures": len(failures),
        "availability": 1.0 - (len(failures) / len(checked) if checked else 1.0),
        "error_rate": len(failures) / len(checked) if checked else 1.0,
        "p95_seconds": p95,
    }


def format_duration(seconds: float) -> str:
    seconds = max(0, int(seconds))
    days, remainder = divmod(seconds, 86_400)
    hours, remainder = divmod(remainder, 3_600)
    minutes, _ = divmod(remainder, 60)
    return f"{days}d {hours}h {minutes}m"


def format_percent(value: float) -> str:
    return f"{value * 100:.3f}%"


def format_seconds(value: float) -> str:
    return f"{value:.3f}s"


def status_for(state: dict[str, Any], strict: bool) -> str:
    if not state.get("completed"):
        return "IN_PROGRESS" if not strict else "NOT_COMPLETE"
    threshold_ok = (
        state.get("availability", 0) >= MIN_UPTIME
        and state.get("error_rate", 1) <= MAX_ERROR_RATE
        and state.get("p95_seconds", math.inf) <= MAX_P95_SECONDS
    )
    return "PASS" if threshold_ok and not state.get("recent_failures") else "FAIL"


def last_hours_results(records: list[dict[str, Any]], hours: float) -> list[dict[str, Any]]:
    cutoff = datetime.now(timezone.utc) - timedelta(hours=hours)
    results: list[dict[str, Any]] = []
    for record in records:
        timestamp = record.get("summary", {}).get("updated_at")
        if timestamp and parse_timestamp(timestamp) >= cutoff:
            results.extend(record.get("results", []))
    return results


def daily_snapshots(records: list[dict[str, Any]]) -> list[tuple[str, dict[str, Any]]]:
    snapshots: list[tuple[str, dict[str, Any]]] = []
    for record in records:
        snapshot = record.get("summary", {}).get("daily_docker_snapshot")
        if snapshot:
            timestamp = str(record.get("summary", {}).get("updated_at", "unknown"))
            snapshots.append((timestamp, snapshot))
    return snapshots


def render_markdown(
    state: dict[str, Any],
    records: list[dict[str, Any]],
    last24: list[dict[str, Any]],
) -> str:
    all_results = [result for record in records for result in record.get("results", [])]
    total = result_stats(all_results)
    recent = result_stats(last24)
    status = status_for(state, strict=False)
    lines = [
        "# M4 7-day Stability Report",
        "",
        f"- Generated: `{utc_now()}`",
        f"- Status: **{status}**",
        f"- Evidence cycles: `{len(records)}`",
        "",
        "| Window | Checks | Failures | Availability | 5xx/error rate | P95 |",
        "|---|---:|---:|---:|---:|---:|",
        f"| Full window | {total['checks']} | {total['failures']} | {format_percent(total['availability'])} | {format_percent(total['error_rate'])} | {format_seconds(total['p95_seconds'])} |",
        f"| Last 24h | {recent['checks']} | {recent['failures']} | {format_percent(recent['availability'])} | {format_percent(recent['error_rate'])} | {format_seconds(recent['p95_seconds'])} |",
        "",
        "## Window",
        "",
        f"- Elapsed: `{format_duration(float(state.get('elapsed_seconds', 0)))}`",
        f"- Target: `{format_duration(float(state.get('target_seconds', 0)))}`",
        f"- Completed: `{bool(state.get('completed'))}`",
        "",
        "## Budgets",
        "",
        f"- Availability >= `{format_percent(MIN_UPTIME)}`",
        f"- 5xx/error rate <= `{format_percent(MAX_ERROR_RATE)}`",
        f"- P95 <= `{format_seconds(MAX_P95_SECONDS)}`",
        "",
        "## Recent Failures",
        "",
    ]
    failures = [item for item in checked_results(all_results) if not item.get("ok")][-20:]
    if not failures:
        lines.append("None.")
    else:
        lines.extend(
            [
                "| Timestamp | Check | Status | Detail |",
                "|---|---|---|---|",
            ]
        )
        for failure in failures:
            detail = str(failure.get("error") or failure.get("detail") or "-").replace("|", "\\|")
            lines.append(
                f"| `{failure.get('ts', 'unknown')}` | `{failure.get('check', 'unknown')}` | {failure.get('status', '-')} | {detail} |"
            )
    lines.extend(["", "## Daily Docker Snapshots", ""])
    snapshots = daily_snapshots(records)[-7:]
    if not snapshots:
        lines.append("No daily snapshot has been emitted yet.")
    else:
        for timestamp, snapshot in snapshots:
            healthy = snapshot.get("healthy_or_successful", 0)
            unhealthy = snapshot.get("unhealthy_or_failed", 0)
            ok = bool(snapshot.get("ok"))
            lines.append(f"- `{timestamp}`: healthy/successful `{healthy}`, unhealthy/failed `{unhealthy}`, ok `{ok}`")
    lines.append("")
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--evidence", default="artifacts/stability/m4-7d.jsonl")
    parser.add_argument("--state", default="artifacts/stability/m4-7d-state.json")
    parser.add_argument("--output", default="artifacts/stability/m4-7d-report.md")
    parser.add_argument(
        "--strict",
        action="store_true",
        help="Exit 1 if the 7-day window is incomplete or thresholds fail.",
    )
    args = parser.parse_args()

    state_path = Path(args.state)
    state: dict[str, Any] = {}
    if state_path.exists():
        state = json.loads(state_path.read_text(encoding="utf-8"))
    else:
        print(f"state file not found: {state_path}", file=sys.stderr)
        return 2

    records = read_evidence(Path(args.evidence))
    last24 = last_hours_results(records, 24)
    report = render_markdown(state, records, last24)
    output = Path(args.output)
    atomic_write(output, report)
    print(output)
    status = status_for(state, strict=args.strict)
    if args.strict and status != "PASS":
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
