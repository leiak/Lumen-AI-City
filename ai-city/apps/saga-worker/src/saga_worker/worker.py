"""Saga Worker: consume saga steps with a shared group and explicit DLQ."""
from __future__ import annotations

import base64
import asyncio
import json
import logging
from typing import Any, Awaitable, Callable

from aiokafka import AIOKafkaConsumer, AIOKafkaProducer


logger = logging.getLogger(__name__)
StepHandler = Callable[[dict[str, Any]], Awaitable[None]]
MAX_STEP_ATTEMPTS = 3


class SagaWorker:
    def __init__(
        self,
        kafka_brokers: str,
        group_id: str = "saga-worker",
        topic: str = "saga.steps",
        dlq_topic: str = "saga.steps.dlq",
        handler: StepHandler | None = None,
    ) -> None:
        self.kafka_brokers = kafka_brokers
        self.group_id = group_id
        self.topic = topic
        self.dlq_topic = dlq_topic
        self.handler = handler or self._default_handler
        self._consumer: AIOKafkaConsumer | None = None
        self._producer: AIOKafkaProducer | None = None
        self._running = asyncio.Event()

    async def start(self) -> None:
        if self._running.is_set():
            return
        self._running.set()
        self._consumer = AIOKafkaConsumer(
            self.topic,
            bootstrap_servers=self.kafka_brokers,
            group_id=self.group_id,
            enable_auto_commit=False,
            auto_offset_reset="earliest",
        )
        self._producer = AIOKafkaProducer(
            bootstrap_servers=self.kafka_brokers,
            acks="all",
            enable_idempotence=True,
            request_timeout_ms=5000,
        )
        await self._consumer.start()
        logger.info("kafka consumer started")
        await self._producer.start()
        logger.info("kafka producer started")
        logger.info("saga-worker started")

        try:
            async for msg in self._consumer:
                try:
                    await self._execute_step(msg.value)
                except Exception as exc:
                    await self._publish_dlq(msg.value, exc)
                    logger.exception(
                        "step moved to DLQ topic=%s partition=%s offset=%s",
                        msg.topic,
                        msg.partition,
                        msg.offset,
                    )
                await self._consumer.commit()
        finally:
            await self.stop()

    async def stop(self) -> None:
        if self._consumer is not None:
            try:
                await self._consumer.stop()
            except Exception:
                logger.exception("failed to stop Kafka consumer")
            self._consumer = None
        if self._producer is not None:
            try:
                await self._producer.stop()
            except Exception:
                logger.exception("failed to stop Kafka producer")
            self._producer = None
        self._running.clear()

    async def _publish_dlq(self, payload: bytes, exc: Exception) -> None:
        if self._producer is None:
            raise RuntimeError("Kafka producer is not started")
        dead_letter = {
            "payload": base64.b64encode(payload).decode("ascii"),
            "error": str(exc),
        }
        await self._producer.send_and_wait(
            self.dlq_topic,
            json.dumps(dead_letter, separators=(",", ":")).encode("utf-8"),
        )

    async def _execute_step(self, payload: bytes) -> None:
        """Execute a step; the final exception is handled by the caller."""
        try:
            event = json.loads(payload.decode("utf-8"))
        except (UnicodeDecodeError, json.JSONDecodeError) as exc:
            raise ValueError("invalid saga step JSON") from exc
        if not isinstance(event, dict):
            raise ValueError("saga step must be a JSON object")
        for attempt in range(1, MAX_STEP_ATTEMPTS + 1):
            try:
                await self.handler(event)
                return
            except Exception as exc:
                if attempt == MAX_STEP_ATTEMPTS:
                    raise
                await asyncio.sleep(min(2 ** attempt, 4))

    @staticmethod
    async def _default_handler(event: dict[str, Any]) -> None:
        logger.info(
            "executing step saga_id=%s step=%s",
            event.get("saga_id"),
            event.get("step") or event.get("action"),
        )
