#!/usr/bin/env python3
"""Run reproducible availability/latency probes for the M4 stability window."""

from __future__ import annotations

import argparse
import json
import os
import statistics
import subprocess
import sys
import time
import urllib.error
import urllib.request
import uuid
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

SEVEN_DAYS_SECONDS = 7 * 24 * 60 * 60
MAX_ERROR_RATE = 0.001
MAX_P95_SECONDS = 2.0
MIN_UPTIME = 0.999
TOKEN_REFRESH_SECONDS = 45 * 60


def utc_now() -> str:
    return datetime.now(timezone.utc).isoformat()


def atomic_write_json(path: Path, value: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    temporary = path.with_suffix(path.suffix + ".tmp")
    temporary.write_text(json.dumps(value, ensure_ascii=False, indent=2), encoding="utf-8")
    temporary.replace(path)


class StabilityProbe:
    def __init__(self, api_url: str, use_docker: bool) -> None:
        self.api_url = api_url.rstrip("/")
        self.use_docker = use_docker
        self.token = ""
        self.token_issued_at = 0.0
        self.results: list[dict[str, Any]] = []

    def request(
        self,
        check: str,
        method: str,
        path: str,
        body: dict[str, Any] | None = None,
        authenticated: bool = False,
    ) -> tuple[bool, float]:
        headers = {"Content-Type": "application/json"}
        if authenticated:
            headers["Authorization"] = f"Bearer {self.token}"
        payload = json.dumps(body).encode() if body is not None else None
        request = urllib.request.Request(
            f"{self.api_url}{path}", data=payload, headers=headers, method=method
        )
        started = time.perf_counter()
        try:
            with urllib.request.urlopen(request, timeout=5) as response:
                response.read()
                elapsed = time.perf_counter() - started
                self.record(check, True, elapsed)
                return True, elapsed
        except urllib.error.HTTPError as error:
            elapsed = time.perf_counter() - started
            error.read()
            self.record(check, False, elapsed, f"http_{error.code}")
            return False, elapsed
        except Exception as error:
            elapsed = time.perf_counter() - started
            self.record(check, False, elapsed, str(error))
            return False, elapsed

    def record(
        self, check: str, ok: bool, elapsed: float, error: str | None = None
    ) -> None:
        result: dict[str, Any] = {
            "ts": utc_now(),
            "check": check,
            "ok": ok,
            "elapsed_seconds": round(elapsed, 6),
        }
        if error:
            result["error"] = error
        self.results.append(result)

    def authenticate(self) -> bool:
        username = os.environ.get("AICITY_STABILITY_USERNAME")
        password = os.environ.get("AICITY_STABILITY_PASSWORD", "stability-password-2026")
        if not username:
            username = f"stability_{uuid.uuid4().hex[:12]}"
        login_body = {"username": username, "password": password}
        ok, _ = self.request("auth.login", "POST", "/v1/auth/login", login_body)
        if not ok:
            register_body = {
                "username": username,
                "password": password,
                "email": f"{username}@stability.local",
                "display_name": "M4 Stability",
            }
            ok, _ = self.request("auth.register", "POST", "/v1/auth/register", register_body)
            if not ok:
                return False
            ok, _ = self.request("auth.login", "POST", "/v1/auth/login", login_body)
        if not ok:
            return False
        response = urllib.request.Request(
            f"{self.api_url}/v1/auth/login",
            data=json.dumps(login_body).encode(),
            headers={"Content-Type": "application/json"},
            method="POST",
        )
        with urllib.request.urlopen(response, timeout=5) as raw:
            payload = json.loads(raw.read().decode())
        self.token = payload.get("token", "")
        self.token_issued_at = time.monotonic()
        return bool(self.token)

    def run_cycle(self) -> None:
        if not self.token or time.monotonic() - self.token_issued_at > TOKEN_REFRESH_SECONDS:
            if not self.authenticate():
                self.record("auth.session", False, 0.0, "no_token")
                return
        self.request("health", "GET", "/health")
        self.request("world.tiles", "GET", "/v1/tiles")
        self.request(
            "npc.talk",
            "POST",
            "/v1/npc/npc_wang_boss_001/talk",
            {"player_id": "m4-stability", "choice_id": "root"},
        )
        self.request("wallet", "GET", "/v1/wallet", authenticated=True)
        self.request("inventory", "GET", "/v1/inventory", authenticated=True)
        self.request("transactions", "GET", "/v1/transactions", authenticated=True)

    def docker_snapshot(self) -> dict[str, Any]:
        if not self.use_docker:
            return {"enabled": False}
        try:
            completed = subprocess.run(
                ["docker", "compose", "ps", "--format", "json"],
                capture_output=True,
                text=True,
                timeout=30,
                check=False,
            )
            healthy = 0
            unhealthy = 0
            for line in completed.stdout.splitlines():
                if not line.strip():
                    continue
                try:
                    item = json.loads(line)
                except json.JSONDecodeError:
                    continue
                status = str(item.get("Status", "")).lower()
                if "healthy" in status or "exited (0)" in status:
                    healthy += 1
                elif "unhealthy" in status or "exited" in status:
                    unhealthy += 1
            return {
                "enabled": True,
                "healthy_or_successful": healthy,
                "unhealthy_or_failed": unhealthy,
                "ok": completed.returncode == 0 and unhealthy == 0,
            }
        except Exception as error:
            return {"enabled": True, "ok": False, "error": str(error)}


def summarize(probe: StabilityProbe, started: float, target_seconds: float) -> dict[str, Any]:
    elapsed = time.monotonic() - started
    checks = [item for item in probe.results if not item["check"].startswith("auth.")]
    failures = [item for item in checks if not item["ok"]]
    recent_failures = failures[-10:]
    latencies = sorted(item["elapsed_seconds"] for item in checks)
    p95 = statistics.quantiles(latencies, n=100, method="inclusive")[94] if len(latencies) >= 20 else max(latencies, default=0.0)
    availability = 1.0 - (len(failures) / len(checks) if checks else 1.0)
    error_rate = len(failures) / len(checks) if checks else 1.0
    return {
        "completed": elapsed >= target_seconds,
        "elapsed_seconds": round(elapsed, 3),
        "target_seconds": target_seconds,
        "checks": len(checks),
        "failures": len(failures),
        "recent_failures": recent_failures,
        "availability": round(availability, 6),
        "error_rate": round(error_rate, 6),
        "p95_seconds": round(p95, 6),
        "thresholds": {
            "max_error_rate": MAX_ERROR_RATE,
            "max_p95_seconds": MAX_P95_SECONDS,
            "min_availability": MIN_UPTIME,
        },
        "passed": (
            elapsed >= target_seconds
            and availability >= MIN_UPTIME
            and error_rate <= MAX_ERROR_RATE
            and p95 <= MAX_P95_SECONDS
        ),
        "updated_at": utc_now(),
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--api-url", default=os.environ.get("AICITY_API_URL", "http://localhost:18080"))
    parser.add_argument("--duration-seconds", type=int, default=SEVEN_DAYS_SECONDS)
    parser.add_argument("--interval-seconds", type=int, default=30)
    parser.add_argument("--output-dir", default="artifacts/stability")
    parser.add_argument("--once", action="store_true")
    parser.add_argument("--no-docker", action="store_true")
    args = parser.parse_args()

    output_dir = Path(args.output_dir)
    output_dir.mkdir(parents=True, exist_ok=True)
    evidence_path = output_dir / "m4-7d.jsonl"
    state_path = output_dir / "m4-7d-state.json"
    probe = StabilityProbe(args.api_url, use_docker=not args.no_docker)
    started = time.monotonic()
    if evidence_path.exists():
        for line in evidence_path.read_text(encoding="utf-8").splitlines():
            if not line.strip():
                continue
            try:
                item = json.loads(line)
            except json.JSONDecodeError:
                continue
            probe.results.extend(item.get("results", []))
        if probe.results:
            try:
                first_result_time = datetime.fromisoformat(probe.results[0]["ts"])
                elapsed_wall = (
                    datetime.now(timezone.utc) - first_result_time
                ).total_seconds()
                started = time.monotonic() - max(0.0, elapsed_wall)
            except (KeyError, TypeError, ValueError):
                pass
    last_daily = 0.0

    try:
        while True:
            cycle_start = len(probe.results)
            probe.run_cycle()
            cycle_results = probe.results[cycle_start:]
            daily_snapshot = None
            if time.monotonic() - last_daily >= 24 * 60 * 60:
                daily_snapshot = probe.docker_snapshot()
                last_daily = time.monotonic()
            summary = summarize(probe, started, args.duration_seconds)
            if daily_snapshot is not None:
                summary["daily_docker_snapshot"] = daily_snapshot
            atomic_write_json(state_path, summary)
            with evidence_path.open("a", encoding="utf-8") as evidence:
                evidence.write(
                    json.dumps(
                        {"summary": summary, "results": cycle_results},
                        ensure_ascii=False,
                    )
                    + "\n"
                )
            print(json.dumps(summary, ensure_ascii=False), flush=True)
            if args.once:
                break
            if summary["completed"]:
                return 0 if summary["passed"] else 1
            time.sleep(max(1, args.interval_seconds))
    except KeyboardInterrupt:
        summary = summarize(probe, started, args.duration_seconds)
        summary["completed"] = False
        summary["interrupted"] = True
        atomic_write_json(state_path, summary)
        print(json.dumps(summary, ensure_ascii=False), flush=True)
        return 1

    summary = summarize(probe, started, args.duration_seconds)
    if args.once:
        return 0 if summary["failures"] == 0 and summary["p95_seconds"] <= MAX_P95_SECONDS else 1
    return 0 if summary["passed"] else 1


if __name__ == "__main__":
    sys.exit(main())
