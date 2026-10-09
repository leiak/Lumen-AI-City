"""Service-only cross-city transfer endpoints."""
from __future__ import annotations

import os
import secrets
from typing import Annotated, Literal

from asyncpg import Pool
from fastapi import APIRouter, Depends, Header, HTTPException, Query

from economy_service.db import get_pool
from economy_service.errors import EconomyError
from economy_service.schemas import (
    CrossCityCreditRequest,
    CrossCityTransferRequest,
    CrossCityTransferResponse,
)
from economy_service.services.cross_city_service import CrossCityService

router = APIRouter(
    prefix="/internal/v1/cross-city-transfers",
    tags=["internal-cross-city-transfers"],
)

SERVICE_TOKEN = os.environ.get("CROSS_CITY_SERVICE_TOKEN", "dev-cross-city-service-token")


def require_service_token(
    authorization: Annotated[str | None, Header()] = None,
) -> None:
    if not authorization or not authorization.startswith("Bearer "):
        raise HTTPException(
            status_code=403,
            detail={
                "code": "CROSS_CITY_SERVICE_AUTH_FAILED",
                "msg": "cross-city service token required",
            },
        )
    token = authorization.removeprefix("Bearer ")
    if not secrets.compare_digest(token, SERVICE_TOKEN):
        raise HTTPException(
            status_code=403,
            detail={
                "code": "CROSS_CITY_SERVICE_AUTH_FAILED",
                "msg": "invalid cross-city service token",
            },
        )


def _error(exc: EconomyError) -> HTTPException:
    return HTTPException(
        status_code=exc.http_status,
        detail={"code": exc.code, "msg": exc.msg},
    )


@router.post("/credit", response_model=CrossCityTransferResponse, status_code=201)
async def credit_transfer(
    body: CrossCityCreditRequest,
    _: Annotated[None, Depends(require_service_token)],
    pool: Annotated[Pool, Depends(get_pool)],
) -> CrossCityTransferResponse:
    service = CrossCityService(pool)
    try:
        transfer = await service.credit_inbound(
            global_id=body.global_id,
            source_city_id=body.source_city_id,
            source_user_id=body.source_user_id,
            destination_city_id=body.destination_city_id,
            destination_user_id=body.destination_user_id,
            currency=body.currency,
            amount=body.amount,
            idempotency_key=body.idempotency_key,
            reserved_at=body.reserved_at,
            expires_at=body.expires_at,
            trace_id=body.trace_id,
        )
    except EconomyError as exc:
        raise _error(exc) from exc
    return CrossCityTransferResponse(**transfer)


@router.post("/reserve", response_model=CrossCityTransferResponse, status_code=201)
async def reserve_transfer(
    body: CrossCityTransferRequest,
    _: Annotated[None, Depends(require_service_token)],
    pool: Annotated[Pool, Depends(get_pool)],
) -> CrossCityTransferResponse:
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
        raise _error(exc) from exc
    return CrossCityTransferResponse(**transfer)


@router.get("/expired", response_model=list[CrossCityTransferResponse])
async def list_expired_transfers(
    _: Annotated[None, Depends(require_service_token)],
    pool: Annotated[Pool, Depends(get_pool)],
    limit: Annotated[int, Query(ge=1, le=100)] = 100,
) -> list[CrossCityTransferResponse]:
    service = CrossCityService(pool)
    try:
        transfers = await service.list_expired_outbound(limit)
    except EconomyError as exc:
        raise _error(exc) from exc
    return [CrossCityTransferResponse(**transfer) for transfer in transfers]


@router.post("/{global_id}/settle", response_model=CrossCityTransferResponse)
async def settle_transfer(
    global_id: str,
    _: Annotated[None, Depends(require_service_token)],
    pool: Annotated[Pool, Depends(get_pool)],
) -> CrossCityTransferResponse:
    service = CrossCityService(pool)
    try:
        transfer = await service.settle_outbound(global_id)
    except EconomyError as exc:
        raise _error(exc) from exc
    return CrossCityTransferResponse(**transfer)


@router.get("/{global_id}", response_model=CrossCityTransferResponse)
async def get_transfer(
    global_id: str,
    _: Annotated[None, Depends(require_service_token)],
    pool: Annotated[Pool, Depends(get_pool)],
    direction: Annotated[
        Literal["outbound", "inbound"],
        Query(),
    ] = "outbound",
) -> CrossCityTransferResponse:
    service = CrossCityService(pool)
    try:
        transfer = (
            await service.get_inbound(global_id)
            if direction == "inbound"
            else await service.get(global_id)
        )
    except EconomyError as exc:
        raise _error(exc) from exc
    if transfer is None:
        raise HTTPException(
            status_code=404,
            detail={
                "code": "CROSS_CITY_SOURCE_NOT_FOUND",
                "msg": "cross-city transfer not found",
            },
        )
    return CrossCityTransferResponse(**transfer)
