from typing import Annotated

from fastapi import APIRouter, Depends, HTTPException

from economy_service.auth import AuthenticatedPlayer, require_roles
from economy_service.db import get_pool
from economy_service.schemas.marketplace_npc import NpcTemplateCreate
from economy_service.services.marketplace_service import MarketplaceService
from economy_service.services.template_validator import BTInvalidError, validate_bt_skeleton

router = APIRouter(prefix="/v1/marketplace/npc-templates", tags=["marketplace"])


@router.post("", status_code=201)
async def create_npc_template(
    body: NpcTemplateCreate,
    player: Annotated[AuthenticatedPlayer, Depends(require_roles({"creator", "admin"}))],
    pool: Annotated[object, Depends(get_pool)],
) -> dict:
    if body.bt_skeleton:
        try:
            validate_bt_skeleton(body.bt_skeleton)
        except BTInvalidError as exc:
            raise HTTPException(
                status_code=400,
                detail={"code": "R_031", "msg": str(exc)},
            ) from exc

    service = MarketplaceService(pool)
    template_id = await service.create_npc_template(
        creator_id=player.id,
        name=body.name,
        ocean_json=body.ocean_json.model_dump(),
        price_gold=body.price_gold,
        avatar_url=body.avatar_url,
        bt_skeleton=body.bt_skeleton,
        product_catalog=(
            [item.model_dump() for item in body.product_catalog]
            if body.product_catalog
            else None
        ),
    )
    return {"id": template_id}


@router.get("")
async def list_npc_templates(
    status: str = "live",
    limit: int = 50,
    offset: int = 0,
    pool: Annotated[object, Depends(get_pool)] = None,
) -> list[dict]:
    service = MarketplaceService(pool)
    return await service.list_npc_templates(status=status, limit=limit, offset=offset)


@router.get("/{template_id}")
async def get_npc_template(
    template_id: int,
    pool: Annotated[object, Depends(get_pool)] = None,
) -> dict:
    service = MarketplaceService(pool)
    template = await service.get_npc_template(template_id)
    if template is None:
        raise HTTPException(
            status_code=404,
            detail={"code": "R_028", "msg": "template not found"},
        )
    return template


@router.post("/{template_id}/take-down")
async def take_down_npc_template(
    template_id: int,
    player: Annotated[AuthenticatedPlayer, Depends(require_roles({"creator", "admin"}))],
    pool: Annotated[object, Depends(get_pool)] = None,
) -> dict:
    service = MarketplaceService(pool)
    template = await service.get_npc_template(template_id)
    if template is None:
        raise HTTPException(
            status_code=404,
            detail={"code": "R_028", "msg": "template not found"},
        )
    if player.role != "admin" and str(template["creator_id"]) != player.id:
        raise HTTPException(
            status_code=403,
            detail={"code": "R_027", "msg": "creator or admin role required"},
        )
    await service.take_down_npc_template(template_id, actor_id=player.id)
    return {"status": "taken_down"}
