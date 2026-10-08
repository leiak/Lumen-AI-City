from datetime import datetime
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field


class OceanJson(BaseModel):
    O: float = Field(ge=0, le=1)  # noqa: E741 — OCEAN domain identifier
    C: float = Field(ge=0, le=1)
    E: float = Field(ge=0, le=1)
    A: float = Field(ge=0, le=1)
    N: float = Field(ge=0, le=1)


class ProductItem(BaseModel):
    name: str = Field(min_length=1, max_length=128)
    price_gold: int = Field(ge=0)
    stock: int | None = Field(default=None, ge=0)


class NpcTemplateCreate(BaseModel):
    name: str = Field(min_length=1, max_length=64)
    avatar_url: str | None = None
    ocean_json: OceanJson
    bt_skeleton: str | None = None
    product_catalog: list[ProductItem] | None = None
    price_gold: int = Field(ge=10)


class NpcTemplateResponse(NpcTemplateCreate):
    model_config = ConfigDict(from_attributes=True)

    id: int
    creator_id: UUID
    status: str
    created_at: datetime
    updated_at: datetime
