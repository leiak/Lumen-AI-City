"""Phase C.2 — Pydantic request/response models for the bt-editor-api HTTP surface.

Keep these thin (no business logic) — validation lives in ``db.validate_tree``
which reuses the C.1 BT loader for shape checks.
"""
from __future__ import annotations

from pydantic import BaseModel, Field


class TreeSummary(BaseModel):
    """List-endpoint item: name + version + last update timestamp."""

    name: str
    version: int
    updated_at: str


class TreeFull(BaseModel):
    """Full row representation used by GET-by-name and POST upsert response."""

    npc_id: str
    name: str
    tree_json: dict
    version: int
    created_at: str
    updated_at: str


class SaveTreeRequest(BaseModel):
    """POST /api/v1/bt/{npc_id}/{tree_name} body."""

    tree_json: dict


class SimulateState(BaseModel):
    """Inner ``state`` payload of a simulate request.

    Mirrors ``BTState`` field-by-field (except ``tick_count`` /
    ``registry`` which the endpoint manages). All fields have defaults so
    the caller can omit any subset.
    """

    player_position: list[int] | None = None
    npc_state: dict = Field(default_factory=dict)
    time_of_day: str = "noon"
    max_ticks: int = Field(default=100, ge=1, le=1000)


class SimulateRequest(BaseModel):
    """POST /api/v1/bt/{npc_id}/{tree_name}/simulate body."""

    tree_json: dict
    state: SimulateState = Field(default_factory=SimulateState)
    tick_limit: int = Field(default=100, ge=1, le=1000)


class SimulateTraceEntry(BaseModel):
    """One tick in the simulate trace log."""

    node_id: str
    status: str
    tick_count: int


class SimulateResponse(BaseModel):
    """Simulate endpoint response — final status + per-tick trace + state."""

    status: str  # "success" | "failure" | "running" | "error"
    trace: list[SimulateTraceEntry]
    final_state: dict


__all__ = [
    "SaveTreeRequest",
    "SimulateRequest",
    "SimulateResponse",
    "SimulateState",
    "SimulateTraceEntry",
    "TreeFull",
    "TreeSummary",
]
