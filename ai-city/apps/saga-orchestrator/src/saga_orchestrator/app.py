"""Saga Orchestrator FastAPI 入口。"""
from fastapi import FastAPI

app = FastAPI(title="AI City - Saga Orchestrator")


@app.get("/health")
async def health() -> dict:
    return {"status": "ok"}


@app.post("/v1/sagas")
async def start_saga(definition: dict) -> dict:
    # TODO: 调用 SagaOrchestrator
    return {"saga_id": "stub"}


@app.get("/v1/sagas/{saga_id}")
async def get_saga(saga_id: str) -> dict:
    return {"saga_id": saga_id, "state": "pending"}


# §32.7 Saga Dashboard 指标
@app.get("/metrics/saga")
async def metrics() -> dict:
    return {
        "running": 0,
        "completed_24h": 0,
        "failed_24h": 0,
        "compensation_rate": 0.0,
        "p99_duration_ms": 0,
    }
