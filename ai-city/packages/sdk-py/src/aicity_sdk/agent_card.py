"""Agent Card 模型。"""
from __future__ import annotations

from typing import Optional

from pydantic import BaseModel, Field


class AgentCard(BaseModel):
    agent_id: str
    name: str
    description: str = ""
    url: str
    provider: str = "external"
    version: str = "0.1.0"
    capabilities: list[str] = Field(default_factory=list)
    auth: Optional[dict] = None
