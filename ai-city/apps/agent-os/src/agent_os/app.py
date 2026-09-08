"""FastAPI app factory + lifespan (Sprint 12 min slice).

Endpoints:
  GET /healthz   → 200 {"status":"ok"}（不依赖 redis 真活着）

Lifespan:
  startup  → RedisPub.ping() (best-effort, warn on error)
            → NpcRegistry.load_dir(config.npc_templates_dir)
            → SayScheduler 启动为 background asyncio.Task
  shutdown → stop event set + task joined + redis 连接 close（如果有 close 方法）

Service 名 print 在 startup banner 用于 docker-compose 日志对账。
"""
from __future__ import annotations

import asyncio
import logging
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from pathlib import Path

from fastapi import FastAPI

from agent_os.action_dispatcher import ActionDispatcher
from agent_os.config import Config
from agent_os.npc_registry import NpcRegistry
from agent_os.redis_pub import RedisPub
from agent_os.say_scheduler import SayScheduler

logger = logging.getLogger(__name__)


def create_app(config: Config | None = None) -> FastAPI:
    cfg = config or Config()
    redis_pub = RedisPub(cfg.redis_url)
    registry = NpcRegistry(Path(cfg.npc_templates_dir))
    dispatcher = ActionDispatcher(redis_pub, channel=cfg.redis_channel_npc_dialogue)
    scheduler = SayScheduler(
        registry=registry,
        dispatcher=dispatcher,  # type: ignore[arg-type]
        tick_seconds=cfg.say_tick_seconds,
    )

    stop_event = asyncio.Event()
    scheduler_task: asyncio.Task[None] | None = None

    @asynccontextmanager
    async def lifespan(app: FastAPI) -> AsyncIterator[None]:
        nonlocal scheduler_task
        logger.info(
            "agent-os starting",
            extra={
                "service": cfg.service_name,
                "http_port": cfg.http_port,
                "redis_channel": cfg.redis_channel_npc_dialogue,
                "tick_seconds": cfg.say_tick_seconds,
                "npc_templates_dir": cfg.npc_templates_dir,
            },
        )
        # best-effort ping（不阻塞 startup；ping 失败时 /healthz 仍 OK）
        try:
            await redis_pub.ping()
        except Exception as e:  # noqa: BLE001
            logger.warning("startup redis ping failed", extra={"err": str(e)})
        # spawn scheduler
        scheduler_task = asyncio.create_task(scheduler.run(stop_event))
        try:
            yield
        finally:
            stop_event.set()
            if scheduler_task is not None:
                try:
                    await asyncio.wait_for(scheduler_task, timeout=5.0)
                except (TimeoutError, asyncio.CancelledError):
                    logger.warning("scheduler did not stop in time")
            # RedisPub 是 fire-and-forget；每 publish 新连接；这里无 close 方法。
            logger.info("agent-os stopped", extra={"service": cfg.service_name})

    app = FastAPI(title=cfg.service_name, lifespan=lifespan)

    @app.get("/healthz")
    async def healthz() -> dict[str, str]:
        return {"status": "ok"}

    # expose for test inspection
    app.state.config = cfg
    app.state.registry = registry
    app.state.dispatcher = dispatcher
    app.state.scheduler = scheduler
    return app
