import base64
import hashlib
import hmac
import json
import os
import time
from dataclasses import dataclass
from typing import Annotated, Any

from asyncpg import Pool
from fastapi import Depends, Header, HTTPException

from economy_service.db import get_pool


@dataclass(frozen=True)
class AuthenticatedPlayer:
    id: str
    username: str
    role: str


class AccessTokenError(Exception):
    pass


def _b64url_decode(value: str) -> bytes:
    padding = "=" * (-len(value) % 4)
    return base64.urlsafe_b64decode(value + padding)


def decode_access_token(token: str) -> dict[str, Any]:
    secret = os.environ.get("JWT_SECRET")
    if not secret:
        raise AccessTokenError("JWT secret is not configured")

    parts = token.split(".")
    if len(parts) != 3:
        raise AccessTokenError("malformed access token")

    header_b64, payload_b64, signature_b64 = parts
    try:
        header = json.loads(_b64url_decode(header_b64))
    except (ValueError, json.JSONDecodeError) as exc:
        raise AccessTokenError("malformed access token") from exc
    if header != {"alg": "HS256", "typ": "JWT"}:
        raise AccessTokenError("unsupported access token algorithm")

    expected = hmac.new(
        secret.encode(),
        f"{header_b64}.{payload_b64}".encode(),
        hashlib.sha256,
    ).digest()
    try:
        actual = _b64url_decode(signature_b64)
    except ValueError as exc:
        raise AccessTokenError("malformed access token") from exc
    if not hmac.compare_digest(actual, expected):
        raise AccessTokenError("invalid access token signature")

    try:
        payload = json.loads(_b64url_decode(payload_b64))
    except (ValueError, json.JSONDecodeError) as exc:
        raise AccessTokenError("malformed access token") from exc
    if not isinstance(payload, dict) or not isinstance(payload.get("sub"), str):
        raise AccessTokenError("access token subject missing")
    exp = payload.get("exp")
    if not isinstance(exp, (int, float)) or exp < time.time() - 30:
        raise AccessTokenError("access token expired")
    return payload


async def get_current_player(
    authorization: Annotated[str | None, Header(alias="Authorization")] = None,
    pool: Annotated[Pool, Depends(get_pool)] = None,
) -> AuthenticatedPlayer:
    if not authorization or not authorization.startswith("Bearer "):
        raise HTTPException(
            status_code=401,
            detail={"code": "R_026", "msg": "valid player token required"},
        )

    try:
        payload = decode_access_token(authorization.removeprefix("Bearer "))
    except AccessTokenError as exc:
        raise HTTPException(
            status_code=401,
            detail={"code": "R_026", "msg": str(exc)},
        ) from exc

    async with pool.acquire() as conn:
        row = await conn.fetchrow(
            "SELECT id, role FROM player WHERE id = $1::uuid",
            payload["sub"],
        )
    if row is None:
        raise HTTPException(
            status_code=401,
            detail={"code": "R_026", "msg": "player not found"},
        )

    return AuthenticatedPlayer(
        id=str(row["id"]),
        username=str(payload.get("uname", "")),
        role=str(row["role"]),
    )


def require_roles(required_roles: set[str]):
    async def dependency(
        player: Annotated[AuthenticatedPlayer, Depends(get_current_player)],
    ) -> AuthenticatedPlayer:
        if player.role not in required_roles:
            required = "creator role required" if "creator" in required_roles else "admin role required"
            raise HTTPException(
                status_code=403,
                detail={"code": "R_027" if "creator" in required_roles else "R_026", "msg": required},
            )
        return player

    return dependency
