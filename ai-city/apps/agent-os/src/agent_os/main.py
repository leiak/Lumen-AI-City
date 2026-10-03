"""uvicorn entry point — `uv run python -m agent_os.main` 或 script `agent-os`."""
from __future__ import annotations

import logging

import uvicorn

from agent_os.config import Config


def main() -> None:
    cfg = Config()
    # Python stdlib `logging.basicConfig(level=...)` 要求大写（"INFO"），与 Go/Rust
    # 的小写约定不一致；uvicorn 也接受大小写，但 lib 严格要求。normalize 一次：
    # - basicConfig 走 UPPER（lib 约束）
    # - uvicorn log_level 走 lower（与 uvicorn 内部常量一致）
    log_level_upper = cfg.log_level.upper()
    logging.basicConfig(
        level=log_level_upper,
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
