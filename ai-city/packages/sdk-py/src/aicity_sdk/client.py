"""A2A 客户端。"""
from __future__ import annotations

import httpx


class A2AClient:
    """A2A 联邦客户端。"""

    def __init__(self, gateway_url: str, api_key: str) -> None:
        self.gateway_url = gateway_url.rstrip("/")
        self.api_key = api_key
        self._client = httpx.AsyncClient(
            base_url=self.gateway_url,
            headers={"Authorization": f"Bearer {api_key}"},
            timeout=10.0,
        )

    async def register_card(self, card: dict) -> dict:
        """注册 Agent Card。"""
        resp = await self._client.post("/v1/cards", json=card)
        resp.raise_for_status()
        return resp.json()

    async def discover(self, capability: str) -> list[dict]:
        """发现同 capability 的 Agent。"""
        resp = await self._client.get("/v1/discover", params={"capability": capability})
        resp.raise_for_status()
        return resp.json()

    async def send_message(self, to_agent_id: str, payload: dict) -> dict:
        """发送消息。"""
        resp = await self._client.post(
            "/v1/messages",
            json={"to_agent_id": to_agent_id, "payload": payload},
        )
        resp.raise_for_status()
        return resp.json()

    async def close(self) -> None:
        await self._client.aclose()
