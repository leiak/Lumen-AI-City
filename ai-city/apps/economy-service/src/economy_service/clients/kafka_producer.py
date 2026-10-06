"""aiokafka fire-and-forget producer for econ.{tx.completed, gold.emitted, gold.sunk}.

Fire-and-forget: 生产失败不阻塞事务 (observability hook 后面接).
"""
from __future__ import annotations
import json
import os
import logging
from typing import Any

logger = logging.getLogger(__name__)

KAFKA_BOOTSTRAP = os.environ.get("KAFKA_BOOTSTRAP_SERVERS", "kafka:9092")
TOPIC_TX_COMPLETED = "econ.tx.completed"
TOPIC_GOLD_EMITTED = "econ.gold.emitted"
TOPIC_GOLD_SUNK = "econ.gold.sunk"


class KafkaProducer:
    """aiokafka 异步 producer 包装; start() 在 lifespan 内调一次."""

    def __init__(self, bootstrap: str = KAFKA_BOOTSTRAP) -> None:
        self.bootstrap = bootstrap
        self._producer = None  # aiokafka.AIOKafkaProducer

    async def start(self) -> None:
        from aiokafka import AIOKafkaProducer  # lazy import for testability
        self._producer = AIOKafkaProducer(
            bootstrap_servers=self.bootstrap,
            value_serializer=lambda v: json.dumps(v, default=str).encode(),
            enable_idempotence=False,  # fire-and-forget, at-most-once OK
        )
        try:
            await self._producer.start()
            logger.info("kafka_producer.started: bootstrap=%s", self.bootstrap)
        except Exception as e:
            # 启动失败不阻塞主服务（fire-and-forget 设计）
            logger.warning("kafka_producer.start failed: %s", e)
            self._producer = None

    async def stop(self) -> None:
        if self._producer is not None:
            try:
                await self._producer.stop()
            except Exception as e:
                logger.warning("kafka_producer.stop failed: %s", e)
            self._producer = None

    async def send(self, topic: str, payload: dict[str, Any]) -> None:
        """Fire-and-forget; 失败仅记日志."""
        if self._producer is None:
            logger.debug("kafka_producer.send skipped (no producer): topic=%s", topic)
            return
        try:
            await self._producer.send_and_wait(topic, payload, timeout=2.0)
        except Exception as e:
            logger.warning("kafka_producer.send failed topic=%s: %s", topic, e)