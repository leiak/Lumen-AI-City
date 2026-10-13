from __future__ import annotations

import asyncio
import contextlib
import logging
import os

from fastapi import FastAPI

from saga_worker.worker import SagaWorker


logger = logging.getLogger(__name__)
logging.basicConfig(level=os.getenv("LOG_LEVEL", "INFO").upper())

WORKER_ID = os.getenv("WORKER_ID", "worker-1")
KAFKA_BROKERS = os.getenv("KAFKA_BROKERS", "kafka:9092")
KAFKA_TOPIC = os.getenv("KAFKA_TOPIC", "saga.steps")
KAFKA_GROUP = os.getenv("KAFKA_GROUP", "saga-worker-group")
KAFKA_DLQ_TOPIC = os.getenv("KAFKA_DLQ_TOPIC", "saga.steps.dlq")

app = FastAPI(title="saga-worker", version="2.1.0")
_worker: SagaWorker | None = None
_task: asyncio.Task[None] | None = None


@app.get("/healthz")
async def healthz():
    return {"status": "ok", "worker_id": WORKER_ID, "topic": KAFKA_TOPIC}


@app.get("/readyz")
async def readyz():
    if _worker is None or _worker._consumer is None or _worker._producer is None:
        return {"status": "not_ready", "topic": KAFKA_TOPIC}
    task_error = None
    if _task is not None and _task.done():
        task_error = repr(_task.exception())
    return {
        "status": "not_ready" if task_error else "ready",
        "worker_id": WORKER_ID,
        "topic": KAFKA_TOPIC,
        "group_id": KAFKA_GROUP,
        "task_error": task_error,
    }


@app.on_event("startup")
async def startup():
    global _worker, _task
    if _worker is None:
        _worker = SagaWorker(
            KAFKA_BROKERS,
            group_id=KAFKA_GROUP,
            topic=KAFKA_TOPIC,
            dlq_topic=KAFKA_DLQ_TOPIC,
        )
        _task = asyncio.create_task(_worker.start(), name="saga-worker-consumer")


@app.on_event("shutdown")
async def shutdown():
    global _worker, _task
    if _task is not None:
        _task.cancel()
        with contextlib.suppress(asyncio.CancelledError):
            await _task
    if _worker is not None:
        await _worker.stop()
    _worker = None
    _task = None
