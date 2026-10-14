from __future__ import annotations

import asyncio
import json
import os
from datetime import datetime
from typing import Any

import psycopg
from pydantic import BaseModel, Field


class AgentMemoryRequest(BaseModel):
    percept: dict[str, Any]
    importance: int = Field(default=3, ge=1, le=5)


class AgentMemoryResult(BaseModel):
    source: str = "episodic"
    id: str | None = None
    content: str
    importance: int | None = None
    metadata: dict[str, Any] = {}
    created_at: datetime | None = None


def pg_connect() -> psycopg.Connection:
    return psycopg.connect(
        host=os.getenv("POSTGRES_HOST", "postgres"),
        port=int(os.getenv("POSTGRES_PORT", "5432")),
        dbname=os.getenv("POSTGRES_DB", "aicity"),
        user=os.getenv("POSTGRES_USER", "aicity"),
        password=os.getenv("POSTGRES_PASSWORD", "aicity_dev"),
    )


def content_from_percept(percept: dict[str, Any]) -> str:
    if isinstance(percept.get("content"), str):
        return percept["content"]
    return json.dumps(percept, ensure_ascii=False)


def remember_sync(agent_id: str, request: AgentMemoryRequest) -> str:
    metadata = request.percept.get("metadata", {})
    if not isinstance(metadata, dict):
        metadata = {"value": metadata}
    with pg_connect() as connection:
        with connection.cursor() as cursor:
            cursor.execute(
                """
                INSERT INTO episodic_memory (agent_id, player_id, content, importance, metadata)
                VALUES (%s, %s, %s, %s, %s::jsonb)
                RETURNING id::text
                """,
                (
                    agent_id,
                    request.percept.get("player_id"),
                    content_from_percept(request.percept),
                    request.importance,
                    json.dumps(metadata, ensure_ascii=False),
                ),
            )
            return cursor.fetchone()[0]


def recall_sync(agent_id: str, query: str, top_k: int) -> list[AgentMemoryResult]:
    search = f"%{query}%"
    with pg_connect() as connection:
        with connection.cursor() as cursor:
            cursor.execute(
                """
                SELECT id::text, content, importance, metadata, created_at
                FROM episodic_memory
                WHERE agent_id = %s AND (%s = '' OR content ILIKE %s)
                ORDER BY importance DESC, created_at DESC
                LIMIT %s
                """,
                (agent_id, query, search, top_k),
            )
            rows = cursor.fetchall()
    return [
        AgentMemoryResult(
            id=row[0],
            content=row[1],
            importance=row[2],
            metadata=row[3],
            created_at=row[4],
        )
        for row in rows
    ]


async def remember(agent_id: str, request: AgentMemoryRequest) -> str:
    return await asyncio.to_thread(remember_sync, agent_id, request)


async def recall(agent_id: str, query: str, top_k: int) -> list[AgentMemoryResult]:
    return await asyncio.to_thread(recall_sync, agent_id, query, top_k)
