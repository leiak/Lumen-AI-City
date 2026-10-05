# ai-city/apps/agent-os/tests/llm/test_litellm_provider.py
import pytest
from agent_os.llm.base import LLMProvider, LLMRequest, LLMResponse

def test_llm_provider_interface():
    # Protocol check
    assert hasattr(LLMProvider, "complete")
    assert hasattr(LLMProvider, "stream")

def test_llm_request_dataclass():
    req = LLMRequest(
        system_prompt="你是一个 NPC",
        messages=[{"role": "user", "content": "你好"}],
        max_tokens=200,
        temperature=0.7,
    )
    assert req.system_prompt == "你是一个 NPC"
    assert req.max_tokens == 200

def test_llm_response_dataclass():
    resp = LLMResponse(
        text="你好！",
        input_tokens=10,
        output_tokens=20,
        finish_reason="stop",
    )
    assert resp.text == "你好！"
