from __future__ import annotations

import base64
import json

from unittest.mock import AsyncMock

import pytest

from saga_worker.worker import SagaWorker


@pytest.mark.asyncio
async def test_execute_step_retries_before_final_error() -> None:
    worker = SagaWorker("unused:9092")
    worker.handler = AsyncMock(side_effect=[RuntimeError("down"), None])
    payload = json.dumps({"saga_id": "s-1", "step": "reserve"}).encode("utf-8")

    await worker._execute_step(payload)

    assert worker.handler.await_count == 2


@pytest.mark.asyncio
async def test_invalid_step_json_fails_without_retry() -> None:
    worker = SagaWorker("unused:9092")
    worker.handler = AsyncMock()

    with pytest.raises(ValueError, match="invalid saga step JSON"):
        await worker._execute_step(b"not-json")

    worker.handler.assert_not_awaited()


@pytest.mark.asyncio
async def test_publish_dlq_wraps_original_payload() -> None:
    worker = SagaWorker("unused:9092")
    worker._producer = AsyncMock()
    payload = b'{"saga_id":"s-1"}'

    await worker._publish_dlq(payload, RuntimeError("handler failed"))

    worker._producer.send_and_wait.assert_awaited_once()
    args = worker._producer.send_and_wait.await_args.args
    assert args[0] == "saga.steps.dlq"
    envelope = json.loads(args[1])
    assert base64.b64decode(envelope["payload"]) == payload
