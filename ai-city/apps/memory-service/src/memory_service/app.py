"""Memory Service FastAPI 入口。"""
from fastapi import FastAPI

app = FastAPI(title="AI City - Memory Service")


@app.get("/health")
async def health() -> dict:
    return {"status": "ok"}


@app.post("/v1/agents/{agent_id}/remember")
async def remember(agent_id: str, percept: dict, importance: int = 3) -> dict:
    # TODO: 调用 MemoryPipeline
    return {"ok": True}


@app.post("/v1/agents/{agent_id}/recall")
async def recall(agent_id: str, query: str, top_k: int = 5) -> dict:
    # TODO: 调用 MemoryPipeline
    return {"results": []}
