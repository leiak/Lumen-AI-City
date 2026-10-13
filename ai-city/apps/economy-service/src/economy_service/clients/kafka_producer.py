"""Reliable async Kafka producer for economy and cross-city events.

send() schedules the broker write as a background task and returns
immediately; broker failures are counted and logged, but never raised to
the caller. Idempotence makes producer retries safe for the same key.
"""
from __future__ import annotations
import asyncio
import json
import os
import logging
from typing import Any

from prometheus_client import Counter

logger = logging.getLogger(__name__)

KAFKA_BOOTSTRAP = os.environ.get("KAFKA_BOOTSTRAP_SERVERS", "kafka:9092")
TOPIC_TX_COMPLETED = "econ.tx.completed"
TOPIC_GOLD_EMITTED = "econ.gold.emitted"
TOPIC_GOLD_SUNK = "econ.gold.sunk"

KAFKA_SEND_TOTAL = Counter(
    "economy_kafka_send_total",
    "Economy Kafka send attempts.",
    ["topic"],
)
KAFKA_SEND_FAILURES_TOTAL = Counter(
    "economy_kafka_send_failures_total",
    "Economy Kafka send failures.",
    ["topic"],
)


class KafkaProducer:
    """aiokafka 异步 producer 包装; start() 在 lifespan 内调一次.

    send() 是真异步 — 后台任务执行 broker write; 不阻塞调用方.
    """

    def __init__(self, bootstrap: str = KAFKA_BOOTSTRAP) -> None:
        self.bootstrap = bootstrap
        self._producer = None  # aiokafka.AIOKafkaProducer

    async def start(self) -> None:
        from aiokafka import AIOKafkaProducer  # lazy import for testability
        self._producer = AIOKafkaProducer(
            bootstrap_servers=self.bootstrap,
            value_serializer=lambda v: json.dumps(v, default=str).encode(),
            acks="all",
            enable_idempotence=True,
            request_timeout_ms=5000,
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

    async def _send_one(self, topic: str, payload: dict[str, Any], key: str | None = None) -> None:
        """后台执行实际 broker write — 失败仅记日志."""
        KAFKA_SEND_TOTAL.labels(topic=topic).inc()
        try:
            await self._producer.send_and_wait(
                topic,
                payload,
                key=None if key is None else key.encode("utf-8"),
                timeout=5.0,
            )
        except Exception as e:
            KAFKA_SEND_FAILURES_TOTAL.labels(topic=topic).inc()
            logger.warning("kafka_producer.send failed topic=%s: %s", topic, e)

    @staticmethod
    def _partition_key(topic: str, payload: dict[str, Any]) -> str | None:
        """Use stable business identifiers so idempotent retries stay ordered."""
        if topic.startswith("econ.crosscity."):
            value = payload.get("global_id")
        elif topic == TOPIC_TX_COMPLETED:
            value = payload.get("transaction_id") or payload.get("global_id")
        elif topic in (TOPIC_GOLD_EMITTED, TOPIC_GOLD_SUNK):
            value = payload.get("player_id") or payload.get("global_id")
        else:
            value = payload.get("idempotency_key") or payload.get("global_id") or payload.get("player_id")
        return str(value) if value is not None else None

    async def send(
        self,
        topic: str,
        payload: dict[str, Any],
        key: str | None = None,
    ) -> None:
        """Fire-and-forget; 后台调度, 立刻返回. 失败仅记日志.

        不阻塞调用方 — 单次 send < 1ms (只 schedule task). 实际 broker write
        在 _send_one 后台协程里跑, 2s timeout 兜底.
        """
        if self._producer is None:
            logger.debug("kafka_producer.send skipped (no producer): topic=%s", topic)
            return
        if key is None:
            key = self._partition_key(topic, payload)
        asyncio.create_task(self._send_one(topic, payload, key))
