#!/usr/bin/env python3
"""Repeatable acceptance check for the cross-city gold transfer slice."""
from __future__ import annotations

import json
import os
import uuid
from urllib.error import HTTPError
from urllib.request import Request, urlopen

import asyncpg

BASE_URL = os.environ.get("CROSS_CITY_ACCEPTANCE_URL", "http://127.0.0.1:8005")
TOKEN = os.environ.get("CROSS_CITY_SERVICE_TOKEN", "dev-cross-city-service-token")
AMOUNT = 77


def request(method: str, path: str, body: dict | None = None) -> dict:
    data = json.dumps(body).encode() if body is not None else None
    req = Request(
        BASE_URL + path,
        data=data,
        method=method,
        headers={
            "Authorization": f"Bearer {TOKEN}",
            "Content-Type": "application/json",
        },
    )
    try:
        with urlopen(req, timeout=10) as response:
            return json.loads(response.read())
    except HTTPError as exc:
        detail = exc.read().decode()
        raise AssertionError(f"{method} {path} failed: {exc.code} {detail}") from exc


def assert_equal(actual, expected, label: str) -> None:
    if actual != expected:
        raise AssertionError(f"{label}: expected {expected}, got {actual}")


async def main() -> None:
    run_id = str(uuid.uuid4()).replace("-", "")
    source_city_id = f"city-a-{run_id[:8]}"
    destination_city_id = f"city-b-{run_id[:8]}"
    source_user_id = f"cross-source-{run_id}"
    destination_user_id = f"cross-destination-{run_id}"
    global_id = str(uuid.uuid4())
    idempotency_key = f"accept-{run_id}"

    conn = await asyncpg.connect(os.environ["DATABASE_URL"])
    try:
        await conn.execute(
            "INSERT INTO wallet (user_id, gold_balance) VALUES ($1, $2)",
            source_user_id,
            1000,
        )

        reserved = request(
            "POST",
            "/internal/v1/cross-city-transfers/reserve",
            {
                "source_city_id": source_city_id,
                "source_user_id": source_user_id,
                "destination_city_id": destination_city_id,
                "destination_user_id": destination_user_id,
                "currency": "gold",
                "amount": AMOUNT,
                "idempotency_key": idempotency_key,
                "trace_id": f"accept-{run_id}",
            },
        )
        global_id = reserved["global_id"]
        assert_equal(reserved["status"], "reserved", "step 1 reserve")

        source_balance = await conn.fetchval(
            "SELECT gold_balance FROM wallet WHERE user_id = $1", source_user_id
        )
        assert_equal(source_balance, 1000 - AMOUNT, "step 1 source debit")

        credited = request(
            "POST",
            "/internal/v1/cross-city-transfers/credit",
            {
                "global_id": global_id,
                "source_city_id": source_city_id,
                "source_user_id": source_user_id,
                "destination_city_id": destination_city_id,
                "destination_user_id": destination_user_id,
                "currency": "gold",
                "amount": AMOUNT,
                "idempotency_key": idempotency_key,
                "trace_id": f"accept-{run_id}",
                "reserved_at": reserved["reserved_at"],
                "expires_at": reserved["expires_at"],
            },
        )
        assert_equal(credited["status"], "credited", "step 2 destination credit")

        settled = request("POST", f"/internal/v1/cross-city-transfers/{global_id}/settle")
        assert_equal(settled["status"], "settled", "step 3 source settle")

        source_balance = await conn.fetchval(
            "SELECT gold_balance FROM wallet WHERE user_id = $1", source_user_id
        )
        assert_equal(source_balance, 1000 - AMOUNT, "step 4 source wallet")
        destination_balance = await conn.fetchval(
            "SELECT gold_balance FROM wallet WHERE user_id = $1", destination_user_id
        )
        assert_equal(destination_balance, AMOUNT, "step 5 destination wallet")

        outbound, inbound = await conn.fetchrow(
            """
            SELECT
              MAX(balance) FILTER (WHERE direction = 'outbound') AS outbound,
              MAX(balance) FILTER (WHERE direction = 'inbound') AS inbound
            FROM bridge_position
            WHERE peer_city_id IN ($1, $2)
            """,
            destination_city_id,
            source_city_id,
        )
        assert_equal(outbound, AMOUNT, "step 6 outbound bridge")
        assert_equal(inbound, -AMOUNT, "step 6 inbound bridge")

        replayed = request(
            "POST",
            "/internal/v1/cross-city-transfers/credit",
            {
                "global_id": global_id,
                "source_city_id": source_city_id,
                "source_user_id": source_user_id,
                "destination_city_id": destination_city_id,
                "destination_user_id": destination_user_id,
                "currency": "gold",
                "amount": AMOUNT,
                "idempotency_key": idempotency_key,
                "trace_id": f"accept-{run_id}",
                "reserved_at": reserved["reserved_at"],
                "expires_at": reserved["expires_at"],
            },
        )
        assert_equal(replayed["status"], "credited", "step 7 duplicate replay")
        replay_balance = await conn.fetchval(
            "SELECT gold_balance FROM wallet WHERE user_id = $1", destination_user_id
        )
        assert_equal(replay_balance, AMOUNT, "step 7 no duplicate credit")

        outbound_status = request("GET", f"/internal/v1/cross-city-transfers/{global_id}")
        inbound_status = request(
            "GET",
            f"/internal/v1/cross-city-transfers/{global_id}?direction=inbound",
        )
        assert_equal(outbound_status["status"], "settled", "step 8 outbound status")
        assert_equal(inbound_status["status"], "credited", "step 8 inbound status")

        print("CROSS_CITY_GOLD_ACCEPTANCE PASS (8/8)")
        print(f"global_id={global_id} source={source_user_id} destination={destination_user_id}")
    finally:
        await conn.close()


if __name__ == "__main__":
    import asyncio

    asyncio.run(main())
