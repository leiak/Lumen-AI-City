from typing import Annotated, Literal

from fastapi import APIRouter, Depends, HTTPException, Query
from pydantic import BaseModel, Field

from economy_service.auth import AuthenticatedPlayer, get_current_player, require_roles
from economy_service.db import get_pool
from economy_service.errors import EconomyError
from economy_service.services.marketplace_service import MarketplaceService

router = APIRouter(prefix="/v1/marketplace", tags=["marketplace"])


class PurchaseRequest(BaseModel):
    template_kind: Literal["npc", "saga"]
    template_id: int = Field(gt=0)
    idempotency_key: str = Field(min_length=1)


@router.post("/purchase")
async def purchase_template(
    body: PurchaseRequest,
    player: Annotated[AuthenticatedPlayer, Depends(get_current_player)],
    pool: Annotated[object, Depends(get_pool)],
) -> dict:
    service = MarketplaceService(pool)
    try:
        purchase_id = await service.purchase_template(
            user_id=player.id,
            template_kind=body.template_kind,
            template_id=body.template_id,
            idempotency_key=body.idempotency_key,
        )
    except EconomyError as exc:
        raise HTTPException(
            status_code=exc.http_status,
            detail={"code": exc.code, "msg": exc.msg},
        ) from exc
    return {"purchase_id": purchase_id}


@router.get("/inventory/{user_id}")
async def list_inventory(
    user_id: str,
    player: Annotated[AuthenticatedPlayer, Depends(get_current_player)],
    limit: int = Query(default=50, ge=1, le=100),
    offset: int = Query(default=0, ge=0),
    pool: Annotated[object, Depends(get_pool)] = None,
) -> list[dict]:
    if player.role != "admin" and player.id != user_id:
        raise HTTPException(
            status_code=403,
            detail={"code": "R_026", "msg": "admin role required"},
        )
    service = MarketplaceService(pool)
    return await service.list_inventory(user_id, limit=limit, offset=offset)


@router.get("/revenue/{creator_id}")
async def list_creator_revenue(
    creator_id: str,
    player: Annotated[AuthenticatedPlayer, Depends(require_roles({"creator", "admin"}))],
    limit: int = Query(default=50, ge=1, le=100),
    offset: int = Query(default=0, ge=0),
    pool: Annotated[object, Depends(get_pool)] = None,
) -> list[dict]:
    if player.role != "admin" and player.id != creator_id:
        raise HTTPException(
            status_code=403,
            detail={"code": "R_027", "msg": "creator role required"},
        )
    service = MarketplaceService(pool)
    return await service.list_creator_revenue(creator_id, limit=limit, offset=offset)
