"""Unit tests for scripts/acceptance_economy_v1.py helpers.

These tests mock urllib.request.urlopen (and the BT action's httpx.Client
patch) so they run without docker / postgres / redis. The acceptance
binary's E2E flow against docker is verified separately by W5.2.
"""
from __future__ import annotations

import json
import sys
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

# Add scripts/ to path so we can import the acceptance module directly.
_SCRIPTS = Path(__file__).resolve().parent.parent / "scripts"
if str(_SCRIPTS) not in sys.path:
    sys.path.insert(0, str(_SCRIPTS))

import urllib.error  # noqa: E402

from acceptance_economy_v1 import (  # noqa: E402
    _check,
    _req,
    step1_register_two_players,
)


def _ok_resp(body: dict | bytes = b"{}") -> MagicMock:
    """Build a MagicMock that quacks like urllib's context-manager response."""
    resp = MagicMock()
    resp.status = 200
    resp.read.return_value = body if isinstance(body, bytes) else json.dumps(body).encode()
    resp.__enter__ = MagicMock(return_value=resp)
    resp.__exit__ = MagicMock(return_value=None)
    return resp


def test_req_get_returns_status_and_body():
    """_req GET parses JSON response into a dict and returns (200, body)."""
    resp = _ok_resp({"gold_balance": 500, "token_balance": 100})
    with patch("urllib.request.urlopen", return_value=resp) as mock_urlopen:
        status, body = _req("GET", "/health")
    assert status == 200
    assert body == {"gold_balance": 500, "token_balance": 100}
    # Method + URL flowed through correctly
    args, _ = mock_urlopen.call_args
    req = args[0]
    assert req.method == "GET"
    assert req.full_url.endswith("/health")


def test_req_post_sends_json_body():
    """_req POST serializes body to JSON and sets Content-Type."""
    resp = _ok_resp(b"{}")
    with patch("urllib.request.urlopen", return_value=resp) as mock_urlopen:
        status, body = _req("POST", "/api/v1/wallet/transfer", {"key": "value"})
    assert status == 200
    args, _ = mock_urlopen.call_args
    req = args[0]
    assert req.method == "POST"
    assert json.loads(req.data) == {"key": "value"}
    assert req.headers.get("Content-type") == "application/json"


def test_req_http_error_returns_error_status_and_body():
    """_req catches urllib HTTPError → (status, json_body)."""
    fp = MagicMock()
    fp.read.return_value = b'{"detail": {"code": "R_022", "msg": "insufficient"}}'
    err = urllib.error.HTTPError(
        "http://example.com/api/v1/wallet/purchase",
        402, "Payment Required", {}, fp,
    )

    with patch("urllib.request.urlopen", side_effect=err):
        status, body = _req("POST", "/api/v1/wallet/purchase", {"x": 1})
    assert status == 402
    assert body["detail"]["code"] == "R_022"
    assert body["detail"]["msg"] == "insufficient"


def test_check_pass_prints_and_returns_true(capsys):
    """_check prints [OK] PASS for True and returns True."""
    result = _check(1, "transfer A→B", True)
    captured = capsys.readouterr()
    assert result is True
    out = captured.out
    assert "[OK]" in out
    assert "PASS" in out
    assert "transfer A→B" in out
    assert "Step  1:" in out  # 2-char wide right-aligned step number


def test_check_fail_prints_and_returns_false(capsys):
    """_check prints [FAIL] FAIL for False with optional detail; returns False."""
    result = _check(7, "idempotent", False, "tx_id mismatch")
    captured = capsys.readouterr()
    assert result is False
    out = captured.out
    assert "[FAIL]" in out
    assert "FAIL" in out
    assert "idempotent" in out
    assert "tx_id mismatch" in out
    # detail suffix uses em-dash separator (—)
    assert "—" in out


def test_step1_returns_two_ids_when_health_ok():
    """step1 returns 2 non-empty distinct IDs when /health=200 and psql works.

    We patch the psql subprocess call (docker compose exec) to return a
    deterministic UUID per username, then assert step1 yields those IDs.
    """
    # /health first → 200
    health_resp = _ok_resp(b'{"status":"ok"}')
    # Two psql subprocess calls → demo UUID then admin UUID
    subprocess_results = [
        MagicMock(returncode=0, stdout="11111111-1111-1111-1111-111111111111", stderr=""),
        MagicMock(returncode=0, stdout="22222222-2222-2222-2222-222222222222", stderr=""),
    ]
    with patch("urllib.request.urlopen", return_value=health_resp), \
         patch("acceptance_economy_v1.subprocess.run", side_effect=subprocess_results):
        alice, bob = step1_register_two_players()

    assert alice == "11111111-1111-1111-1111-111111111111"
    assert bob == "22222222-2222-2222-2222-222222222222"
    assert alice != bob


# ---------------------------------------------------------------------------
# Bonus: env-var override path + service-down path (defensive coverage).
# These don't count toward the 6 required tests; they lock down edge cases
# that bit us in past acceptance scripts.
# ---------------------------------------------------------------------------


def test_step1_uses_env_override_without_calling_subprocess():
    """If ALICE_PLAYER_ID/BOB_PLAYER_ID env vars are set, step1 uses them."""
    import os

    health_resp = _ok_resp(b'{"status":"ok"}')
    with patch.dict(os.environ, {
        "ALICE_PLAYER_ID": "env-alice",
        "BOB_PLAYER_ID": "env-bob",
    }), patch("urllib.request.urlopen", return_value=health_resp), \
         patch("acceptance_economy_v1.subprocess.run") as mock_run:
        alice, bob = step1_register_two_players()
    assert alice == "env-alice"
    assert bob == "env-bob"
    # psql was NOT invoked when env override was present
    assert mock_run.call_count == 0


def test_step1_returns_empty_when_health_unreachable():
    """If /health doesn't return 200, step1 returns ('', '') without psql."""
    fp = MagicMock()
    fp.read.return_value = b'{"detail":"down"}'
    err = urllib.error.HTTPError(
        "http://localhost:8005/health", 503, "Service Unavailable", {}, fp,
    )
    with patch("urllib.request.urlopen", side_effect=err), \
         patch("acceptance_economy_v1.subprocess.run") as mock_run:
        alice, bob = step1_register_two_players()
    assert alice == "" and bob == ""
    assert mock_run.call_count == 0
