"""FastAPI 入口。"""
from contextlib import asynccontextmanager

from fastapi import FastAPI

from agent_os.config import settings


@asynccontextmanager
async def lifespan(app: FastAPI):
    # 启动逻辑
    print(f"agent-os starting (log_level={settings.log_level})")
    yield
    # 关闭逻辑
    print("agent-os shutting down")


app = FastAPI(title="AI City - Agent OS", lifespan=lifespan)


@app.get("/health")
async def health() -> dict:
    return {"status": "ok", "service": settings.service_name}


@app.post("/v1/agents/{agent_id}/decide")
async def decide(agent_id: str, percepts: list[dict]) -> dict:
    """外部触发一次决策（用于调试与单步执行）。"""
    # TODO: 调用 AgentRuntime
    return {"trace_id": "stub", "decision": {"action": "noop"}}
