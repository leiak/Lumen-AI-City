# apps/economy-service/src/economy_service/api/v1/admin.py
from __future__ import annotations
import os
import secrets
from fastapi import APIRouter, Header, HTTPException
from economy_service.db import get_pool
from economy_service.schemas import SinkRequest
from economy_service.services.central_bank import CentralBankService

router = APIRouter(prefix="/api/v1/admin", tags=["admin"])

ADMIN_TOKEN = os.environ.get("ADMIN_TOKEN", "dev-admin-token")


def _check_admin(authorization: str | None) -> None:
    if not authorization or not authorization.startswith("Bearer "):
        raise HTTPException(403, detail={"code": "R_026", "msg": "admin token required"})
    token = authorization.removeprefix("Bearer ")
    if not secrets.compare_digest(token, ADMIN_TOKEN):
        raise HTTPException(403, detail={"code": "R_026", "msg": "invalid admin token"})


@router.post("/central-bank/emit")
async def admin_emit(authorization: str | None = Header(None)) -> dict:
    _check_admin(authorization)
    svc = CentralBankService(await get_pool())
    return await svc.emit(reason="admin_manual_emit")


@router.post("/central-bank/sink")
async def admin_sink(
    body: SinkRequest, authorization: str | None = Header(None),
) -> dict:
    _check_admin(authorization)
    svc = CentralBankService(await get_pool())
    try:
        return await svc.sink(body.user_id, body.amount, reason=body.reason)
    except ValueError as e:
        raise HTTPException(400, detail={"code": "R_018", "msg": str(e)})
