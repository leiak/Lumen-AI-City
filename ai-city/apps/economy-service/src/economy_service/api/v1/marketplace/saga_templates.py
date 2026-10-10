from typing import Annotated

from fastapi import APIRouter, Depends, HTTPException

from economy_service.auth import AuthenticatedPlayer, require_roles
from economy_service.db import get_pool
from economy_service.schemas.marketplace_saga import SagaTemplateCreate
from economy_service.services.marketplace_service import MarketplaceService
from economy_service.services.template_validator import YamlInvalidError, validate_saga_yaml

router = APIRouter(prefix="/v1/marketplace/saga-templates", tags=["marketplace"])


@router.post("", status_code=201)
async def create_saga_template(
    body: SagaTemplateCreate,
    player: Annotated[AuthenticatedPlayer, Depends(require_roles({"creator", "admin"}))],
    pool: Annotated[object, Depends(get_pool)],
) -> dict:
    try:
        validate_saga_yaml(body.yaml_content)
    except YamlInvalidError as exc:
        raise HTTPException(
            status_code=400,
            detail={"code": "R_032", "msg": str(exc)},
        ) from exc

    service = MarketplaceService(pool)
    template_id = await service.create_saga_template(
        creator_id=player.id,
        name=body.name,
        price_gold=body.price_gold,
        yaml_content=body.yaml_content,
        semantic_version=body.semantic_version,
        icon_url=body.icon_url,
        description=body.description,
        npc_deps=body.npc_deps,
    )
    return {"id": template_id}


@router.get("")
async def list_saga_templates(
    status: str = "live",
    limit: int = 50,
    offset: int = 0,
    pool: Annotated[object, Depends(get_pool)] = None,
) -> list[dict]:
    service = MarketplaceService(pool)
    return await service.list_saga_templates(status=status, limit=limit, offset=offset)


@router.get("/{template_id}")
async def get_saga_template(
    template_id: int,
    pool: Annotated[object, Depends(get_pool)] = None,
) -> dict:
    service = MarketplaceService(pool)
    template = await service.get_saga_template(template_id)
    if template is None:
        raise HTTPException(
            status_code=404,
            detail={"code": "R_028", "msg": "template not found"},
        )
    return template


@router.post("/{template_id}/take-down")
async def take_down_saga_template(
    template_id: int,
    player: Annotated[AuthenticatedPlayer, Depends(require_roles({"creator", "admin"}))],
    pool: Annotated[object, Depends(get_pool)] = None,
) -> dict:
    service = MarketplaceService(pool)
    template = await service.get_saga_template(template_id)
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
    await service.take_down_saga_template(template_id, actor_id=player.id)
    return {"status": "taken_down"}
