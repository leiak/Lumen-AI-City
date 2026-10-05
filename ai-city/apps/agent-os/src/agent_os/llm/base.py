# ai-city/apps/agent-os/src/agent_os/llm/base.py
from abc import ABC, abstractmethod
from dataclasses import dataclass, field
from typing import List, Dict, Literal

@dataclass
class LLMRequest:
    system_prompt: str
    messages: List[Dict[str, str]]
    max_tokens: int = 200
    temperature: float = 0.7

@dataclass
class LLMResponse:
    text: str
    input_tokens: int
    output_tokens: int
    finish_reason: Literal["stop", "length", "error"]

class LLMProvider(ABC):
    @abstractmethod
    async def complete(self, req: LLMRequest) -> LLMResponse: ...
    @abstractmethod
    async def stream(self, req: LLMRequest): ...
