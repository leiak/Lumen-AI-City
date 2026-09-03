"""Saga DSL 运行时：执行 CompiledSaga + Sandbox。"""
from __future__ import annotations

import asyncio
import logging
from dataclasses import dataclass

from saga_dsl.compiler import CompiledSaga

logger = logging.getLogger(__name__)


@dataclass
class SagaContext:
    """Saga 执行上下文。"""
    saga_id: str
    trace_id: str
    params: dict
    state: dict = None

    def __post_init__(self):
        if self.state is None:
            self.state = {}


class SagaRuntime:
    """Saga 运行时：执行 step + 失败时倒序补偿。"""

    def __init__(self, saga: CompiledSaga, action_registry: dict) -> None:
        self.saga = saga
        self.action_registry = action_registry

    async def run(self, ctx: SagaContext) -> bool:
        """执行整个 saga，返回是否成功。"""
        completed: list[str] = []
        try:
            for step in self.saga.steps:
                logger.info(f"saga {ctx.saga_id} executing step {step.name}")
                await self._execute_step(step, ctx)
                completed.append(step.name)
            return True
        except Exception as e:
            logger.exception(f"saga {ctx.saga_id} failed at step, compensating: {e}")
            await self._compensate(completed, ctx)
            return False

    async def _execute_step(self, step, ctx: SagaContext) -> None:
        handler = self.action_registry.get(step.action)
        if not handler:
            raise ValueError(f"unknown action: {step.action}")
        await handler(ctx, step)

    async def _compensate(self, completed: list[str], ctx: SagaContext) -> None:
        """倒序补偿已完成步骤。"""
        for step_name in reversed(completed):
            comp = self.saga.compensations.get(step_name)
            if comp:
                logger.info(f"saga {ctx.saga_id} compensating {step_name}")
                await self._execute_step(comp, ctx)
