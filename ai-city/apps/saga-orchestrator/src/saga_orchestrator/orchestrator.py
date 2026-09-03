"""Saga 编排器：状态机 + 补偿。"""
from __future__ import annotations

import logging
from enum import Enum
from uuid import UUID, uuid4

logger = logging.getLogger(__name__)


class SagaState(str, Enum):
    PENDING = "pending"
    RUNNING = "running"
    COMPENSATING = "compensating"
    COMPLETED = "completed"
    FAILED = "failed"


class SagaOrchestrator:
    def __init__(self) -> None:
        self.sagas: dict[UUID, dict] = {}

    async def start_saga(self, definition: dict) -> UUID:
        saga_id = uuid4()
        self.sagas[saga_id] = {
            "id": saga_id,
            "state": SagaState.PENDING,
            "steps": definition.get("steps", []),
            "completed_steps": [],
            "compensations": [],
        }
        logger.info(f"saga {saga_id} started")
        return saga_id

    async def compensate(self, saga_id: UUID, failed_step: str) -> None:
        """倒序执行已完成的补偿。"""
        saga = self.sagas.get(saga_id)
        if not saga:
            return
        saga["state"] = SagaState.COMPENSATING

        for step in reversed(saga["completed_steps"]):
            comp = step.get("compensation")
            if comp:
                logger.info(f"saga {saga_id} compensating {step['name']}")
                # TODO: 调用 saga-worker 执行补偿
                saga["compensations"].append(step["name"])

        saga["state"] = SagaState.FAILED
