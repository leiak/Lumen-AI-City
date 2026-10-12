#!/usr/bin/env python3
"""GA load test: 50 players + 10 agents against the API gateway."""

from __future__ import annotations

import argparse
import asyncio
import base64
import hashlib
import hmac
import json
import os
import statistics
import time
import uuid

import httpx


def sign_token(subject: str, secret: str) -> str:
    now = int(time.time())
    header = base64.urlsafe_b64encode(json.dumps({"alg": "HS256", "typ": "JWT"}).encode()).rstrip(b"=")
    payload = base64.urlsafe_b64encode(
        json.dumps({"sub": subject, "iat": now, "exp": now + 3600}).encode()
    ).rstrip(b"=")
    signature = hmac.new(secret.encode(), header + b"." + payload, hashlib.sha256).digest()
    sig = base64.urlsafe_b64encode(signature).rstrip(b"=")
    return f"{header.decode()}.{payload.decode()}.{sig.decode()}"


async def worker(
    client: httpx.AsyncClient,
    kind: str,
    username: str,
    password: str,
    stop_at: float,
    latencies: list[float],
    errors: list[str],
) -> None:
    token = ""
    player_id = f"loadtest_{kind}_{username}"
    headers: dict[str, str] = {}
    use_auth = os.environ.get("AICITY_LOADTEST_AUTH", "0") == "1"

    while time.monotonic() < stop_at:
        start = time.perf_counter()
        try:
            if not use_auth and not token:
                token = sign_token(username, os.environ.get("AICITY_JWT_SECRET", "dev-secret-change-me"))
                headers = {"Authorization": f"Bearer {token}"}
            elif use_auth and not token:
                r = await client.post(
                    "/v1/auth/login",
                    json={"username": username, "password": password},
                )
                if r.status_code == 200:
                    token = r.json().get("token", "")
                    headers = {"Authorization": f"Bearer {token}"}
                elif r.status_code == 401:
                    r = await client.post(
                        "/v1/auth/register",
                        json={
                            "username": username,
                            "email": f"{username}@loadtest.dev",
                            "password": password,
                            "display_name": kind,
                        },
                    )
                    if r.status_code == 201:
                        continue
                    raise RuntimeError(f"register failed: {r.status_code} {r.text[:120]}")
                else:
                    raise RuntimeError(f"login failed: {r.status_code} {r.text[:120]}")

            r = await client.post(
                "/v1/world/move",
                headers=headers,
                json={
                    "player_id": player_id,
                    "from_tile_id": "tile_0_0",
                    "to_tile_id": "tile_0_1",
                    "x": 10,
                    "y": 110,
                },
            )
            if r.status_code != 200:
                raise RuntimeError(f"move failed: {r.status_code} {r.text[:120]}")

            r = await client.get("/v1/tiles", headers=headers)
            if r.status_code != 200:
                raise RuntimeError(f"tiles failed: {r.status_code} {r.text[:120]}")

            r = await client.post(
                "/v1/npc/npc_wang_boss_001/talk",
                headers=headers,
                json={
                    "npc_id": "npc_wang_boss_001",
                    "player_id": player_id,
                    "choice_id": "ask_food",
                },
            )
            if r.status_code != 200:
                raise RuntimeError(f"talk failed: {r.status_code} {r.text[:120]}")

            if kind == "player":
                r = await client.get("/v1/products/npc_wang_boss_001", headers=headers)
                if r.status_code != 200:
                    raise RuntimeError(f"products failed: {r.status_code} {r.text[:120]}")

            latencies.append(time.perf_counter() - start)
            elapsed = time.perf_counter() - start
            await asyncio.sleep(max(0.0, 2.5 - elapsed))
        except Exception as exc:  # noqa: BLE001
            errors.append(str(exc))
            await asyncio.sleep(0.1)


async def run(args: argparse.Namespace) -> int:
    limits = httpx.Limits(max_connections=args.players + args.agents + 10)
    timeout = httpx.Timeout(args.timeout)
    run_id = uuid.uuid4().hex[:8]
    password = "loadtest-pass-123"

    async with httpx.AsyncClient(base_url=args.api_url, timeout=timeout, limits=limits) as client:
        health = await client.get("/health")
        if health.status_code != 200:
            print(f"api not healthy: {health.status_code} {health.text}")
            return 2

        latencies: list[float] = []
        errors: list[str] = []
        stop_at = time.monotonic() + args.duration
        tasks = []
        for i in range(args.players):
            username = f"ga_player_{run_id}_{i:03d}"
            tasks.append(asyncio.create_task(worker(client, "player", username, password, stop_at, latencies, errors)))
        for i in range(args.agents):
            username = f"ga_agent_{run_id}_{i:03d}"
            tasks.append(asyncio.create_task(worker(client, "agent", username, password, stop_at, latencies, errors)))

        await asyncio.gather(*tasks)

    total_ops = len(latencies) + len(errors)
    error_rate = (len(errors) / total_ops) if total_ops else 0
    p50 = statistics.median(latencies) if latencies else 0
    p95 = statistics.quantiles(latencies, n=20)[18] if len(latencies) >= 20 else max(latencies, default=0)
    p99 = statistics.quantiles(latencies, n=100)[98] if len(latencies) >= 100 else max(latencies, default=0)
    rps = total_ops / args.duration

    print("==> GA load test")
    print(f"api_url={args.api_url}")
    print(f"players={args.players} agents={args.agents} duration_s={args.duration}")
    print("auth_mode=local-jwt" if os.environ.get("AICITY_LOADTEST_AUTH", "0") != "1" else "auth_mode=api-login")
    print(f"total_ops={total_ops} success={len(latencies)} errors={len(errors)}")
    print(f"rps={rps:.2f} p50={p50:.3f}s p95={p95:.3f}s p99={p99:.3f}s error_rate={error_rate:.2%}")
    if errors:
        print("first_errors:")
        for err in errors[:5]:
            print(f"  - {err}")

    ok = error_rate <= args.max_error_rate and p95 <= args.max_p95
    print(f"verdict={'PASS' if ok else 'FAIL'} max_error_rate={args.max_error_rate:.2%} max_p95={args.max_p95:.2f}s")
    return 0 if ok else 1


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--api-url", default=os.environ.get("AICITY_API_URL", "http://localhost:18080"))
    parser.add_argument("--players", type=int, default=50)
    parser.add_argument("--agents", type=int, default=10)
    parser.add_argument("--duration", type=float, default=60.0)
    parser.add_argument("--timeout", type=float, default=5.0)
    parser.add_argument("--max-error-rate", type=float, default=0.01)
    parser.add_argument("--max-p95", type=float, default=2.0)
    return parser.parse_args()


if __name__ == "__main__":
    raise SystemExit(asyncio.run(run(parse_args())))
