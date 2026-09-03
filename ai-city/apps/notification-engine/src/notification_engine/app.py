"""Notification Engine FastAPI 入口。"""
from fastapi import FastAPI

app = FastAPI(title="AI City - Notification Engine")


@app.get("/health")
async def health() -> dict:
    return {"status": "ok"}


@app.post("/v1/notifications")
async def send_notification(payload: dict) -> dict:
    # TODO: 调用 NotificationDispatcher
    return {"ok": True}
