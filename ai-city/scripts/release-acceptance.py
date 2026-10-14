#!/usr/bin/env python3
"""Run the production release acceptance gate and write auditable evidence."""

from __future__ import annotations

import argparse
import json
import os
import shutil
import subprocess
import sys
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Any


SKIP_CHOICES = ("backend", "frontend", "config", "e2e", "stability", "integrity")


@dataclass(frozen=True)
class Step:
    name: str
    command: list[str]
    cwd: Path
    enabled: bool = True


def utc_timestamp() -> str:
    return time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())


def executable(command: list[str]) -> list[str]:
    resolved = shutil.which(command[0])
    if resolved is None:
        return command
    return [resolved, *command[1:]]


def run_step(step: Step, dry_run: bool) -> dict[str, Any]:
    command = executable(step.command)
    result: dict[str, Any] = {
        "name": step.name,
        "cwd": str(step.cwd),
        "command": command,
        "enabled": step.enabled,
        "skipped": not step.enabled,
    }
    if not step.enabled:
        return result
    if dry_run:
        result["status"] = "dry_run"
        return result

    started = time.monotonic()
    try:
        completed = subprocess.run(
            command,
            cwd=step.cwd,
            text=True,
            encoding="utf-8",
            errors="replace",
            capture_output=True,
            check=False,
            timeout=3600,
        )
        result.update(
            {
                "status": "pass" if completed.returncode == 0 else "fail",
                "returncode": completed.returncode,
                "elapsed_seconds": round(time.monotonic() - started, 3),
                "stdout_tail": completed.stdout[-16_000:],
                "stderr_tail": completed.stderr[-16_000:],
            }
        )
    except (OSError, subprocess.SubprocessError) as error:
        result.update(
            {
                "status": "fail",
                "returncode": None,
                "elapsed_seconds": round(time.monotonic() - started, 3),
                "stderr_tail": str(error),
            }
        )
    return result


def frontend_steps(root: Path, enabled: bool) -> list[Step]:
    steps: list[Step] = []
    for package_dir in (root / "web", root / "apps" / "admin-portal"):
        if not (package_dir / "package.json").exists():
            continue
        label = package_dir.name
        steps.extend(
            [
                Step(f"frontend:{label}:lint", ["npm", "run", "lint"], package_dir, enabled),
                Step(f"frontend:{label}:typecheck", ["npm", "run", "typecheck"], package_dir, enabled),
                Step(f"frontend:{label}:test", ["npm", "test", "--", "--run"], package_dir, enabled),
                Step(f"frontend:{label}:build", ["npm", "run", "build"], package_dir, enabled),
            ]
        )
    return steps


def build_steps(args: argparse.Namespace, root: Path) -> list[Step]:
    python = sys.executable
    stability_script = root / "scripts" / "stability-report.py"
    integrity_script = root / "scripts" / "stability-integrity.py"
    e2e_script = root / "scripts" / "e2e-golden-paths.py"
    scale_compose = [
        "docker",
        "compose",
        "-f",
        "docker-compose.yml",
        "-f",
        "docker-compose.scale.yml",
        "config",
        "--quiet",
    ]

    steps: list[Step] = []
    for go_mod in sorted((root / "apps").glob("*/go.mod")):
        steps.append(
            Step(
                f"backend:{go_mod.parent.name}",
                ["go", "test", "./..."],
                go_mod.parent,
                "backend" not in args.skip,
            )
        )
    steps.extend(frontend_steps(root, "frontend" not in args.skip))

    steps.append(
        Step(
            "config:base",
            ["docker", "compose", "config", "--quiet"],
            root,
            "config" not in args.skip,
        )
    )
    steps.append(
        Step(
            "config:stability-profile",
            ["docker", "compose", "--profile", "stability", "config", "--quiet"],
            root,
            "config" not in args.skip,
        )
    )
    steps.append(
        Step("config:scale-overlay", scale_compose, root, "config" not in args.skip)
    )
    production_env = root / ".env.production"
    if production_env.exists():
        steps.append(
            Step(
                "config:production-overlay",
                [
                    "docker",
                    "compose",
                    "--env-file",
                    ".env.production",
                    "-f",
                    "docker-compose.yml",
                    "-f",
                    "docker-compose.prod.yml",
                    "config",
                    "--quiet",
                ],
                root,
                "config" not in args.skip,
            )
        )

    if e2e_script.exists():
        e2e_command = [python, str(e2e_script)]
        steps.append(
            Step("e2e:golden-paths", e2e_command, root, "e2e" not in args.skip)
        )

    if stability_script.exists():
        steps.append(
            Step(
                "stability:7d-strict",
                [python, str(stability_script), "--strict"],
                root,
                "stability" not in args.skip,
            )
        )
    if integrity_script.exists():
        steps.append(
            Step(
                "integrity:migrations-kafka-backup",
                [python, str(integrity_script), "--once", "--require-backup"],
                root,
                "integrity" not in args.skip,
            )
        )
    return steps


def write_report(path: Path, payload: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--root", default=".", help="ai-city repository root")
    parser.add_argument("--report", default="artifacts/release/acceptance.json")
    parser.add_argument("--api-url", default=os.environ.get("AICITY_API_URL", ""))
    parser.add_argument("--dry-run", action="store_true")
    parser.add_argument("--list-steps", action="store_true")
    parser.add_argument("--skip", action="append", default=[], choices=SKIP_CHOICES)
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    if args.api_url:
        os.environ["AICITY_API_URL"] = args.api_url
    root = Path(args.root).resolve()
    if not root.exists():
        print(f"root not found: {root}", file=sys.stderr)
        return 2

    steps = build_steps(args, root)
    if args.list_steps:
        for step in steps:
            state = "skipped" if not step.enabled else "enabled"
            print(f"{state:7} {step.name}: {' '.join(step.command)} ({step.cwd})")
        return 0

    results = [run_step(step, args.dry_run) for step in steps if step.enabled]
    failed = [item for item in results if item["status"] == "fail"]
    payload = {
        "started_at": utc_timestamp(),
        "root": str(root),
        "passed": not failed,
        "summary": {
            "total_enabled": len(results),
            "passed": sum(item["status"] == "pass" for item in results),
            "failed": len(failed),
        },
        "failed_steps": [item["name"] for item in failed],
        "steps": results,
    }
    report_path = Path(args.report)
    if not report_path.is_absolute():
        report_path = root / report_path
    write_report(report_path, payload)
    payload["finished_at"] = utc_timestamp()
    write_report(report_path, payload)
    print(report_path)
    for item in failed:
        print(f"FAILED {item['name']}", file=sys.stderr)
    return 0 if payload["passed"] else 1


if __name__ == "__main__":
    sys.exit(main())
