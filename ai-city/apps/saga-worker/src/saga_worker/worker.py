"""Saga Worker 主体：从 Kafka 消费 step 任务并执行。"""
from __future__ import annotations

import asyncio
import logging

from aiokafka import AIOKafkaConsumer
from tenacity import retry, stop_after_attempt, wait_exponential

logger = logging.getLogger(__name__)


class SagaWorker:
    def __init__(self, kafka_brokers: str, group_id: str = "saga-worker") -> None:
        self.kafka_brokers = kafka_brokers
        self.group_id = group_id
        self._consumer: AIOKafkaConsumer | None = None

    async def start(self) -> None:
        self._consumer = AIOKafkaConsumer(
            "saga.steps",
            bootstrap_servers=self.kafka_brokers,
            group_id=self.group_id,
            enable_auto_commit=False,
        )
        await self._consumer.start()
        logger.info("saga-worker started")

        try:
            async for msg in self._consumer:
                try:
                    await self._execute_step(msg.value)
                    await self._consumer.commit()
                except Exception:
                    logger.exception("step failed, will retry or DLQ")
                    # TODO: 失败超过 N 次进 DLQ
        finally:
            await self._consumer.stop()

    @retry(stop=stop_after_attempt(3), wait=wait_exponential(multiplier=1, min=2, max=10))
    async def _execute_step(self, payload: bytes) -> None:
        """执行单步（带重试）。"""
        # TODO: 解析 payload 并调用对应 handler
        logger.info(f"executing step: {payload[:64]}...")


async def main() -> None:
    import os

    brokers = os.getenv("KAFKA_BROKERS", "localhost:9092")
    worker = SagaWorker(brokers)
    await worker.start()


if __name__ == "__main__":
    asyncio.run(main())
