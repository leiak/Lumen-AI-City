from pydantic import BaseModel, Field


class SagaTemplateCreate(BaseModel):
    name: str = Field(min_length=1, max_length=64)
    icon_url: str | None = None
    description: str | None = None
    yaml_content: str = Field(min_length=1)
    npc_deps: list[str] = Field(default_factory=list)
    semantic_version: str = Field(pattern=r"^\d+\.\d+\.\d+$")
