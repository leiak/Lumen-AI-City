"""BT Editor FastAPI 入口。"""
from fastapi import FastAPI

app = FastAPI(title="AI City - BT Editor API")


@app.get("/health")
async def health() -> dict:
    return {"status": "ok"}


@app.post("/v1/bt/validate")
async def validate(tree: dict) -> dict:
    """校验 BT JSON Schema。"""
    return {"valid": True, "errors": []}


@app.post("/v1/bt/save")
async def save(tree: dict, version: str = "v1") -> dict:
    """保存 BT（带版本号）。"""
    return {"bt_id": "stub", "version": version}


@app.post("/v1/bt/{bt_id}/simulate")
async def simulate(bt_id: str, scenario: dict) -> dict:
    """沙箱模拟执行。"""
    return {"bt_id": bt_id, "trace": []}
