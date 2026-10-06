"""FastAPI app factory + lifespan (Sprint 12 min slice + Sprint 13 player listener).

Endpoints:
  GET /healthz   → 200 {"status":"ok"}（不依赖 redis 真活着）
  GET /v1/npc/sessions/{sid}/buffer?from_idx=N → 重连补帧（stage 3 / C2 follow-up）

Lifespan:
  startup  → RedisPub.ping() (best-effort, warn on error)
            → NpcRegistry.load_dir(config.npc_templates_dir)
            → SayScheduler 启动为 background asyncio.Task
            → RedisSub 订阅 aicity:player:moved 喂 WelcomeEngine（background task）
            → B2: optional PG pool + EmotionRepository + MemoryWriter（仅当 PG_DSN）
            → B2: 把 emotion_repo/settings 注入 dispatcher（注入失败回落到无注入）
  shutdown → stop event set + task joined + 取消 welcome pump + 关 PG pool

Service 名 print 在 startup banner 用于 docker-compose 日志对账。
"""
from __future__ import annotations

import asyncio
import logging
import os
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from pathlib import Path

import asyncpg
from fastapi import FastAPI, HTTPException, Query

from agent_os.action_dispatcher import ActionDispatcher
from agent_os.config import Config
from agent_os.dispatcher import ActionDispatcher as StreamDispatcher
from agent_os.emotion.repository import EmotionRepository
from agent_os.emotion.settings import EmotionSettings
from agent_os.errors import R015SessionNotFound
from agent_os.memory.writer import MemoryWriter
from agent_os.move_scheduler import MoveScheduler
from agent_os.npc_registry import NpcRegistry
from agent_os.player_listener import PlayerListener
from agent_os.redis_pub import RedisPub
from agent_os.redis_sub import RedisSub
from agent_os.say_scheduler import SayScheduler
from agent_os.stream.session_store import SessionStore
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
    # B2-T08: 2.0 stream dispatcher（与 1.0 dispatcher 共存；后者用于 SayScheduler
    # / WelcomeEngine 的 say() 协议，前者用于 future REST /v1/npc/say_stream 入口）
    stream_dispatcher = StreamDispatcher()
    session_store = SessionStore()  # stage 3 = 重连补帧 60min TTL 内存 store
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

    # B2-T08: load emotion settings eagerly (so app.state.emotion_settings is
    # always set, even when PG_DSN is absent). PG_DSN gates the actual repo/writer.
    emotion_settings = EmotionSettings.from_env()
    pg_dsn = os.getenv("PG_DSN", "").strip()

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
                "pg_dsn_set": bool(pg_dsn),
                "emotion_inject_enabled": emotion_settings.inject_enabled,
            },
        )
        # best-effort ping（不阻塞 startup；ping 失败时 /healthz 仍 OK）
        try:
            await redis_pub.ping()
        except Exception as e:  # noqa: BLE001
            logger.warning("startup redis ping failed", extra={"err": str(e)})

        # B2-T08: optional PG pool + EmotionRepository + MemoryWriter.
        # Best-effort: PG down → warn + leave repo=None (no-injection fallback).
        pg_pool: asyncpg.Pool | None = None
        emotion_repo: EmotionRepository | None = None
        memory_writer: MemoryWriter | None = None
        if pg_dsn:
            try:
                pg_pool = await asyncpg.create_pool(pg_dsn, min_size=1, max_size=4)
                emotion_repo = EmotionRepository(pg_pool)
                memory_writer = MemoryWriter(pg_pool)
                # Wire repo + settings into the 2.0 stream dispatcher so
                # say_stream() picks them up. The 1.0 dispatcher (action_dispatcher.py)
                # is untouched — it doesn't have say_stream.
                stream_dispatcher.emotion_repo = emotion_repo
                stream_dispatcher.emotion_settings = emotion_settings
                stream_dispatcher.memory_writer = memory_writer
                logger.info(
                    "PG pool + EmotionRepository + MemoryWriter initialized",
                    extra={"inject_enabled": emotion_settings.inject_enabled},
                )
            except Exception as e:  # noqa: BLE001
                logger.warning(
                    "PG pool init failed (emotion persistence disabled): %s", e,
                )
                pg_pool = None

        # Expose PG wiring on app.state for test inspection (None = degraded).
        app.state.emotion_repo = emotion_repo
        app.state.memory_writer = memory_writer
        app.state.pg_pool = pg_pool

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
            # B2-T08: close PG pool last (after all background tasks are stopped).
            if pg_pool is not None:
                try:
                    await pg_pool.close()
                except Exception as e:  # noqa: BLE001
                    logger.warning("PG pool close failed: %s", e)
            # RedisPub 是 fire-and-forget；每 publish 新连接；这里无 close 方法。
            logger.info("agent-os stopped", extra={"service": cfg.service_name})

    app = FastAPI(title=cfg.service_name, lifespan=lifespan)

    @app.get("/healthz")
    async def healthz() -> dict[str, str]:
        return {"status": "ok"}

    @app.get("/v1/npc/sessions/{sid}/buffer")
    async def get_session_buffer(
        sid: str,
        from_idx: int = Query(0, ge=0, description="从第 N 个 beat 开始返（含），用于补帧"),
    ) -> dict[str, object]:
        """重连补帧端点（C2 follow-up / stage 3）。

        返回 ``from_idx`` 起的所有 beat（含）+ ``complete`` 标记 + ``sentence_count``。
        session 不存在或 60min 过期 → 400 R_015。
        """
        store: SessionStore = app.state.session_store
        try:
            beats = store.get_buffer(sid, from_idx=from_idx)
        except R015SessionNotFound:
            # 用 R015SessionNotFound.code/message 当真值，
            # 避免 errors.py 改 message 时 HTTP 响应漂移（review issue #4）
            raise HTTPException(
                status_code=R015SessionNotFound.http_status,
                detail={
                    "code": R015SessionNotFound.code,
                    "message": R015SessionNotFound.message,
                    "session_id": sid,
                },
            )
        # sentence_count = store 里 beat 总数（含 from_idx 之前的）
        total_beats = len(store.get_buffer(sid, from_idx=0))
        return {
            "session_id": sid,
            "beats": beats,
            "complete": store.is_complete(sid),
            "sentence_count": total_beats,
        }

    # expose for test inspection
    app.state.config = cfg
    app.state.registry = registry
    app.state.dispatcher = dispatcher
    # B2-T08: 2.0 stream dispatcher (the one with say_stream + emotion_repo wiring).
    app.state.stream_dispatcher = stream_dispatcher
    app.state.scheduler = scheduler
    app.state.player_listener = listener
    app.state.welcome_engine = welcome_engine
    app.state.move_scheduler = move_scheduler
    app.state.session_store = session_store
    # B2-T08: expose emotion wiring on app.state. emotion_repo is None when
    # PG_DSN is unset or pool init failed (graceful degradation).
    app.state.emotion_settings = emotion_settings
    return app
