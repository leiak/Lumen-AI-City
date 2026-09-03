"""Economy Service FastAPI 入口。"""
from fastapi import FastAPI

app = FastAPI(title="AI City - Economy Service")


@app.get("/health")
async def health() -> dict:
    return {"status": "ok"}


@app.get("/v1/wallets/{user_id}")
async def get_wallet(user_id: str) -> dict:
    return {"user_id": user_id, "balance": 0}


@app.post("/v1/transactions")
async def create_transaction(payload: dict) -> dict:
    return {"tx_id": "stub"}
