"""LLM 调用包装（LiteLLM + 多 Provider fallback）。"""
from __future__ import annotations

import logging
from typing import Any

import httpx
from tenacity import retry, stop_after_attempt, wait_exponential

from agent_os.config import settings

logger = logging.getLogger(__name__)


class LLMClient:
    """LLM 客户端：主 Provider + 多路 fallback（§34.4）。"""

    def __init__(self) -> None:
        self.base_url = settings.litellm_base_url
        self.primary = settings.primary_model
        self.fallback = settings.fallback_model

    @retry(
        stop=stop_after_attempt(3),
        wait=wait_exponential(multiplier=1, min=2, max=10),
    )
    async def complete(
        self,
        messages: list[dict[str, str]],
        model: str | None = None,
        max_tokens: int = 1024,
        temperature: float = 0.7,
    ) -> dict[str, Any]:
        """调用 LLM 完成。"""
        model = model or self.primary
        async with httpx.AsyncClient(timeout=30.0) as client:
            resp = await client.post(
                f"{self.base_url}/v1/chat/completions",
                json={
                    "model": model,
                    "messages": messages,
                    "max_tokens": max_tokens,
                    "temperature": temperature,
                },
                headers={
                    "Authorization": f"Bearer {settings.anthropic_api_key}",
                },
            )
            resp.raise_for_status()
            return resp.json()

    async def complete_with_fallback(
        self, messages: list[dict[str, str]], **kwargs: Any
    ) -> dict[str, Any]:
        """主 Provider 失败时自动 fallback。"""
        try:
            return await self.complete(messages, model=self.primary, **kwargs)
        except Exception as e:
            logger.warning(f"primary LLM failed, fallback to {self.fallback}: {e}")
            return await self.complete(messages, model=self.fallback, **kwargs)
