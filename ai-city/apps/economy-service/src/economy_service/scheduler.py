# apps/economy-service/src/economy_service/scheduler.py
"""Async 中央银行 scheduler — 启动期 task，每 EMIT_INTERVAL_HOURS 跑一次。"""
from __future__ import annotations
import asyncio
import os
import logging

from economy_service.db import get_pool
from economy_service.services.central_bank import CentralBankService

logger = logging.getLogger(__name__)
INTERVAL_HOURS = int(os.environ.get("EMIT_INTERVAL_HOURS", "6"))


async def emit_loop() -> None:
    while True:
        try:
            svc = CentralBankService(await get_pool())
            result = await svc.emit(reason="scheduled_emit")
            logger.info("central_bank.emit: %s", result)
        except Exception as e:
            logger.exception("central_bank.emit failed: %s", e)

        await asyncio.sleep(INTERVAL_HOURS * 3600)


def start_scheduler() -> asyncio.Task:
    return asyncio.create_task(emit_loop())
