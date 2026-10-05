# ai-city/apps/agent-os/src/agent_os/llm/cost.py
import os

# Claude Sonnet 4.6 pricing (per 1M tokens)
INPUT_COST = 3.0
OUTPUT_COST = 15.0
MONTHLY_BUDGET_USD = float(os.getenv("LLM_MONTHLY_BUDGET_USD", "100"))


class CostTracker:
    def __init__(self):
        self.total_input_tokens = 0
        self.total_output_tokens = 0

    def record(self, input_tokens: int, output_tokens: int):
        self.total_input_tokens += input_tokens
        self.total_output_tokens += output_tokens

    def cost_usd(self) -> float:
        return (
            self.total_input_tokens / 1e6 * INPUT_COST +
            self.total_output_tokens / 1e6 * OUTPUT_COST
        )

    def over_budget(self) -> bool:
        return self.cost_usd() > MONTHLY_BUDGET_USD