# ai-city/apps/saga-worker/src/saga_worker/main.py
from fastapi import FastAPI
from kafka import KafkaConsumer
import json
import asyncio
import os
import time

app = FastAPI(title="saga-worker", version="2.0.0")

WORKER_ID = os.getenv("WORKER_ID", "worker-1")
KAFKA_BROKERS = os.getenv("KAFKA_BROKERS", "kafka:9092")
TOPIC = os.getenv("KAFKA_TOPIC", "saga.npc.action")


@app.get("/healthz")
async def healthz():
    return {"status": "ok", "worker_id": WORKER_ID}


@app.on_event("startup")
async def startup():
    asyncio.create_task(consume_loop())


async def consume_loop():
    consumer = await asyncio.to_thread(
        KafkaConsumer,
        TOPIC,
        bootstrap_servers=KAFKA_BROKERS.split(","),
        group_id=f"saga-worker-{WORKER_ID}",
        auto_offset_reset="earliest",
    )
    while True:
        msg = await asyncio.to_thread(next, consumer)
        event = json.loads(msg.value.decode())
        await process_event(event)


async def process_event(event):
    """幂等：检查 idempotency_key (TODO: Redis SETNX dedup)"""
    idem_key = f"{event['saga_id']}:{event.get('worker_id', '')}"
    npc_id = event["npc_id"]
    action = event["action"]
    payload = event["payload"]
    # 失败注入：如果 payload 含 force_fail=true，跳过并 publish 补偿
    if payload.get("force_fail"):
        await publish_compensation(event)
        return
    # 模拟执行：调 agent-os 或更新 DB
    await asyncio.sleep(0.5)
    print(f"worker {WORKER_ID} processed {npc_id} {action}")


async def publish_compensation(event):
    """发布补偿事件"""
    print(f"worker {WORKER_ID} triggered compensation for {event['saga_id']}")
    # TODO: publish to saga.compensation topic
