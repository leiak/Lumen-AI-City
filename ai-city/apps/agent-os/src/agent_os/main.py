"""uvicorn entry point — `uv run python -m agent_os.main` 或 script `agent-os`."""
from __future__ import annotations

import logging

import uvicorn

from agent_os.config import Config


def main() -> None:
    cfg = Config()
    logging.basicConfig(
        level=cfg.log_level,
        format="%(asctime)s %(levelname)s %(name)s %(message)s",
    )
    uvicorn.run(
        "agent_os.app:create_app",
        factory=True,
        host="0.0.0.0",
        port=cfg.http_port,
        log_level=cfg.log_level.lower(),
    )


if __name__ == "__main__":
    main()
