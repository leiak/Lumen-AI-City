"""FastAPI app factory + lifespan (Sprint 12 min slice + Sprint 13 player listener).

Endpoints:
  GET /healthz   → 200 {"status":"ok"}（不依赖 redis 真活着）

Lifespan:
  startup  → RedisPub.ping() (best-effort, warn on error)
            → NpcRegistry.load_dir(config.npc_templates_dir)
            → SayScheduler 启动为 background asyncio.Task
            → RedisSub 订阅 aicity:player:moved 喂 WelcomeEngine（background task）
  shutdown → stop event set + task joined + 取消 welcome pump

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
from agent_os.move_scheduler import MoveScheduler
from agent_os.npc_registry import NpcRegistry
from agent_os.player_listener import PlayerListener
from agent_os.redis_pub import RedisPub
from agent_os.redis_sub import RedisSub
from agent_os.say_scheduler import SayScheduler
from agent_os.welcome_engine import WelcomeEngine

logger = logging.getLogger(__name__)


async def _welcome_pump(redis_sub: RedisSub, channel: str, engine: WelcomeEngine) -> None:
    """订阅 player:moved 并逐条喂 WelcomeEngine；task 由 shutdown 取消。"""
    async for payload in redis_sub.messages(channel):
        try:
            await engine.handle_payload(payload)
        except Exception as e:  # noqa: BLE001
            logger.warning("welcome pump error", extra={"err": str(e)})


def create_app(config: Config | None = None) -> FastAPI:
    cfg = config or Config()
    redis_pub = RedisPub(cfg.redis_url)
    registry = NpcRegistry(Path(cfg.npc_templates_dir))
    dispatcher = ActionDispatcher(redis_pub, channel=cfg.redis_channel_npc_dialogue)
    listener = PlayerListener()
    scheduler = SayScheduler(
        registry=registry,
        dispatcher=dispatcher,  # type: ignore[arg-type]
        tick_seconds=cfg.say_tick_seconds,
        listener=listener,
    )
    welcome_engine = WelcomeEngine(
        registry=registry,
        dispatcher=dispatcher,  # type: ignore[arg-type]
        listener=listener,
    )
    move_scheduler = MoveScheduler(
        registry=registry,
        publisher=redis_pub,
        channel=cfg.redis_channel_npc_moved,
        tick_seconds=cfg.move_tick_seconds,
    )
    redis_sub = RedisSub(cfg.redis_url)

    stop_event = asyncio.Event()
    scheduler_task: asyncio.Task[None] | None = None
    welcome_task: asyncio.Task[None] | None = None
    move_task: asyncio.Task[None] | None = None

    @asynccontextmanager
    async def lifespan(app: FastAPI) -> AsyncIterator[None]:
        nonlocal scheduler_task, welcome_task, move_task
        logger.info(
            "agent-os starting",
            extra={
                "service": cfg.service_name,
                "http_port": cfg.http_port,
                "redis_channel": cfg.redis_channel_npc_dialogue,
                "redis_channel_player_moved": cfg.redis_channel_player_moved,
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
        # spawn player_moved → welcome pump（Redis 不可达时仅重连告警，不影响启动）
        welcome_task = asyncio.create_task(
            _welcome_pump(redis_sub, cfg.redis_channel_player_moved, welcome_engine)
        )
        # spawn npc move scheduler（NPC 行为引擎：每 move_tick 发一条 npc_moved）
        move_task = asyncio.create_task(move_scheduler.run(stop_event))
        try:
            yield
        finally:
            stop_event.set()
            if scheduler_task is not None:
                try:
                    await asyncio.wait_for(scheduler_task, timeout=5.0)
                except (TimeoutError, asyncio.CancelledError):
                    logger.warning("scheduler did not stop in time")
            if welcome_task is not None:
                welcome_task.cancel()
                try:
                    await welcome_task
                except asyncio.CancelledError:
                    pass
            if move_task is not None:
                move_task.cancel()
                try:
                    await move_task
                except asyncio.CancelledError:
                    pass
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
    app.state.player_listener = listener
    app.state.welcome_engine = welcome_engine
    app.state.move_scheduler = move_scheduler
    return app
