# apps/economy-service/src/economy_service/api/v1/products.py
from __future__ import annotations
from fastapi import APIRouter

from economy_service.db import get_pool
from economy_service.schemas import Product
from economy_service.services.purchase_service import PurchaseService

router = APIRouter(prefix="/api/v1/products", tags=["products"])


@router.get("/{npc_id}", response_model=list[Product])
async def list_products(npc_id: str) -> list[Product]:
    svc = PurchaseService(await get_pool())
    rows = await svc.list_for_npc(npc_id)
    return [Product(**r, enabled=True) for r in rows]
