"""Cross-city gold transfer public API."""
from __future__ import annotations

from typing import Annotated

from asyncpg import Pool
from fastapi import APIRouter, Depends, HTTPException

from economy_service.auth import AuthenticatedPlayer, get_current_player, require_roles
from economy_service.db import get_pool
from economy_service.errors import EconomyError
from economy_service.schemas import CrossCityTransferRequest, CrossCityTransferResponse
from economy_service.services.cross_city_service import CrossCityService

router = APIRouter(prefix="/api/v1/cross-city-transfers", tags=["cross-city-transfers"])


@router.post("", response_model=CrossCityTransferResponse, status_code=201)
async def reserve_transfer(
    body: CrossCityTransferRequest,
    player: Annotated[AuthenticatedPlayer, Depends(get_current_player)],
    pool: Annotated[Pool, Depends(get_pool)],
) -> CrossCityTransferResponse:
    if body.source_user_id != player.id:
        raise HTTPException(
            status_code=403,
            detail={"code": "CROSS_CITY_VALIDATION_FAILED", "msg": "source user must match token"},
        )
    service = CrossCityService(pool)
    try:
        transfer = await service.reserve(
            source_city_id=body.source_city_id,
            source_user_id=body.source_user_id,
            destination_city_id=body.destination_city_id,
            destination_user_id=body.destination_user_id,
            currency=body.currency,
            amount=body.amount,
            idempotency_key=body.idempotency_key,
            trace_id=body.trace_id,
        )
    except EconomyError as exc:
        raise HTTPException(
            status_code=exc.http_status,
            detail={"code": exc.code, "msg": exc.msg},
        ) from exc
    return CrossCityTransferResponse(**transfer)


@router.get("/{global_id}", response_model=CrossCityTransferResponse)
async def get_transfer(
    global_id: str,
    player: Annotated[AuthenticatedPlayer, Depends(get_current_player)],
    pool: Annotated[Pool, Depends(get_pool)],
) -> CrossCityTransferResponse:
    service = CrossCityService(pool)
    try:
        transfer = await service.get(global_id)
    except EconomyError as exc:
        raise HTTPException(
            status_code=exc.http_status,
            detail={"code": exc.code, "msg": exc.msg},
        ) from exc
    if transfer is None:
        raise HTTPException(
            status_code=404,
            detail={"code": "CROSS_CITY_SOURCE_NOT_FOUND", "msg": "cross-city transfer not found"},
        )
    if player.role != "admin" and player.id not in {
        transfer["source_user_id"],
        transfer["destination_user_id"],
    }:
        raise HTTPException(
            status_code=403,
            detail={"code": "CROSS_CITY_VALIDATION_FAILED", "msg": "transfer owner required"},
        )
    return CrossCityTransferResponse(**transfer)


@router.post("/{global_id}/refund", response_model=CrossCityTransferResponse)
async def refund_transfer(
    global_id: str,
    _: Annotated[AuthenticatedPlayer, Depends(require_roles({"admin"}))],
    pool: Annotated[Pool, Depends(get_pool)],
) -> CrossCityTransferResponse:
    service = CrossCityService(pool)
    try:
        transfer = await service.refund(global_id)
    except EconomyError as exc:
        raise HTTPException(
            status_code=exc.http_status,
            detail={"code": exc.code, "msg": exc.msg},
        ) from exc
    return CrossCityTransferResponse(**transfer)
