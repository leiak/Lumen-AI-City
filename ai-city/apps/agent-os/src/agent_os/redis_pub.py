"""agent-os → Redis publisher（手写 RESP，1.0 不引 redis-py）。

与 apps/world-engine/src/redis_pub.rs 同款契约：
- publish fire-and-forget（失败仅 warn）
- 6 原子计数器（lock-free，dataclass 字段 + 单线程访问即可）
- 1s connect timeout
"""
from __future__ import annotations

import asyncio
import logging
from dataclasses import dataclass
from urllib.parse import urlparse

logger = logging.getLogger(__name__)


@dataclass
class RedisStats:
    messages_published: int = 0
    connect_errors: int = 0
    write_errors: int = 0
    flush_errors: int = 0
    ping_success: int = 0
    ping_failure: int = 0


def parse_redis_addr(url: str) -> tuple[str, int, int | None]:
    """redis://host:port[/db] → (host, port, db_or_None).

    与 world-engine parse_redis_addr 语义一致：
    - 无 auth（user:pass 段在 1.0 不用）
    - 端口缺失 → ValueError
    """
    u = urlparse(url)
    if u.scheme not in ("redis", "rediss"):
        raise ValueError(f"unsupported scheme: {u.scheme}")
    if not u.hostname or u.port is None:
        raise ValueError(f"missing host/port: {url}")
    db: int | None = None
    if u.path and u.path != "/":
        try:
            db = int(u.path.lstrip("/"))
        except ValueError:
            raise ValueError(f"invalid db index: {u.path}")
    return u.hostname, u.port, db


def _format_publish(channel: str, payload: str) -> bytes:
    """Serialize a PUBLISH command in RESP.

    RESP array: *3\r\n$7\r\nPUBLISH\r\n$<len>\r\n<channel>\r\n$<len>\r\n<payload>\r\n
    """
    payload_bytes = payload.encode("utf-8")
    return (
        f"*3\r\n$7\r\nPUBLISH\r\n"
        f"${len(channel)}\r\n{channel}\r\n"
        f"${len(payload_bytes)}\r\n".encode("utf-8")
        + payload_bytes
        + b"\r\n"
    )


def _format_ping() -> bytes:
    return b"*1\r\n$4\r\nPING\r\n"


class RedisPub:
    """fire-and-forget publisher。"""

    def __init__(self, url: str, *, connect_timeout: float = 1.0) -> None:
        self._host, self._port, self._db = parse_redis_addr(url)
        self._url = url
        self._timeout = connect_timeout
        self._stats = RedisStats()

    def stats(self) -> RedisStats:
        return self._stats

    async def publish(self, channel: str, payload: str) -> None:
        """发送 PUBLISH。失败仅 warn，1.0 接受丢消息。"""
        cmd = _format_publish(channel, payload)
        try:
            reader, writer = await asyncio.open_connection(
                self._host, self._port, limit=64 * 1024
            )
        except (OSError, asyncio.TimeoutError) as e:
            self._stats.connect_errors += 1
            logger.warning(
                "redis connect failed", extra={"host": self._host, "port": self._port, "err": str(e)}
            )
            return

        try:
            writer.write(cmd)
            await asyncio.wait_for(writer.drain(), timeout=self._timeout)
            self._stats.messages_published += 1
        except (OSError, asyncio.TimeoutError) as e:
            self._stats.write_errors += 1
            logger.warning("redis write failed", extra={"channel": channel, "err": str(e)})
        finally:
            try:
                writer.close()
                await asyncio.wait_for(writer.wait_closed(), timeout=0.5)
            except (OSError, asyncio.TimeoutError):
                self._stats.flush_errors += 1

    async def ping(self) -> bool:
        """独立短连接 PING。用于 /healthz。"""
        try:
            reader, writer = await asyncio.open_connection(
                self._host, self._port, limit=64
            )
        except (OSError, asyncio.TimeoutError):
            self._stats.ping_failure += 1
            return False
        try:
            writer.write(_format_ping())
            await asyncio.wait_for(writer.drain(), timeout=self._timeout)
            line = await asyncio.wait_for(reader.readline(), timeout=self._timeout)
            ok = line == b"+PONG\r\n"
            if ok:
                self._stats.ping_success += 1
            else:
                self._stats.ping_failure += 1
            return ok
        except (OSError, asyncio.TimeoutError):
            self._stats.ping_failure += 1
            return False
        finally:
            try:
                writer.close()
                await asyncio.wait_for(writer.wait_closed(), timeout=0.5)
            except (OSError, asyncio.TimeoutError):
                pass
