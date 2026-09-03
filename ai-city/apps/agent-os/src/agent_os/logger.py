"""单行 JSON 决策日志。

详细设计见 docs/10-低成本规则.md §47 + docs/11-技术细节与玩法模式.md §B.2。
"""
from __future__ import annotations

import json
import logging
import time
from typing import Any

logger = logging.getLogger(__name__)


class DecisionLogger:
    """决策日志：单行 JSON 写到 Loki + Kafka。"""

    def __init__(self, service_name: str = "agent-os") -> None:
        self.service = service_name

    def emit(
        self,
        event_type: str,  # perception | planning | execution | reflection | error
        trace_id: str,
        agent_id: str | None = None,
        **payload: Any,
    ) -> None:
        log_line = {
            "ts": int(time.time() * 1000),
            "service": self.service,
            "trace_id": trace_id,
            "agent_id": agent_id,
            "type": event_type,
            **payload,
        }
        logger.info(json.dumps(log_line, ensure_ascii=False))
        # TODO: 异步发送到 Loki / Kafka
