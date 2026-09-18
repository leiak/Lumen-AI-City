"""Redis SUBSCRIBE 订阅（手写 RESP，不引 redis-py，与 redis_pub 同风格）。

只支持 `message`（普通 PUBLISH）推送；经 Redis Pub/Sub 抵达的首条
`subscribe`/`unsubscribe` 确认会被跳过。连接断开自动重连，退避 1s→5s。
I/O 薄、纯逻辑（iter_messages）可离线单测（喂 asyncio.StreamReader）。
"""
from __future__ import annotations

import asyncio
import logging
from collections.abc import AsyncIterator

from agent_os.redis_pub import parse_redis_addr

logger = logging.getLogger(__name__)

_CONFIRM = {b"subscribe", b"unsubscribe", b"psubscribe", b"punsubscribe"}


def _encode_subscribe(channel: str) -> bytes:
    """SUBSCRIBE <channel> → RESP 数组：*2\\r\\n$9\\r\\nSUBSCRIBE\\r\\n$<len>\\r\\n<channel>\\r\\n。"""
    body = channel.encode("utf-8")
    return f"*2\r\n$9\r\nSUBSCRIBE\r\n${len(body)}\r\n".encode() + body + b"\r\n"


async def _read_bulk(reader: asyncio.StreamReader) -> bytes | None:
    """读一个 RESP bulk string；EOF/错误返回 None。"""
    line = await reader.readline()
    if not line or not line.startswith(b"$"):
        return None
    try:
        length = int(line[1:-2])
    except ValueError:
        return None
    if length < 0:
        return b""
    data = await reader.readexactly(length)
    await reader.readexactly(2)  # 吃掉 trailing CRLF
    return data


async def iter_messages(reader: asyncio.StreamReader) -> AsyncIterator[tuple[str, str]]:
    """读 SUBSCRIBE 推送：对每条 `message` yield (channel, payload)。

    跳过 subscribe/unsubscribe 确认与不认识的类型（如 pmessage）。
    EOF（连接断开）时正常返回 —— 调用方负责重连。
    """
    while True:
        line = await reader.readline()
        if not line:
            return
        if not line.startswith(b"*"):
            continue
        try:
            count = int(line[1:-2])
        except ValueError:
            return
        elems: list[bytes] = []
        for _ in range(count):
            item = await _read_bulk(reader)
            if item is None:
                return
            elems.append(item)
        if not elems:
            continue
        if elems[0] in _CONFIRM:
            continue
        if elems[0] == b"message" and len(elems) == 3:
            channel = elems[1].decode("utf-8", "replace")
            payload = elems[2].decode("utf-8", "replace")
            yield channel, payload


class RedisSub:
    """长连接订阅者；`messages(channel)` 产出一条条 payload，断开自动重连。"""

    def __init__(self, url: str, *, connect_timeout: float = 1.0) -> None:
        self._host, self._port, self._db = parse_redis_addr(url)
        self._timeout = connect_timeout

    async def messages(self, channel: str) -> AsyncIterator[str]:
        """产出收到的 payload；连接断裂自动重连（1s→5s 退避）。永不自行返回。"""
        backoff = 1.0
        while True:
            try:
                reader, writer = await asyncio.open_connection(
                    self._host, self._port, limit=64 * 1024
                )
            except (TimeoutError, OSError):
                logger.warning(
                    "redis subscribe connect failed",
                    extra={"host": self._host, "port": self._port, "err": ""},
                )
                await asyncio.sleep(backoff)
                backoff = min(backoff * 2, 5.0)
                continue
            try:
                writer.write(_encode_subscribe(channel))
                await asyncio.wait_for(writer.drain(), timeout=self._timeout)
                backoff = 1.0  # 连接成功即重置退避
                async for target, payload in iter_messages(reader):
                    if target == channel:
                        yield payload
            except (TimeoutError, OSError) as e:
                logger.warning("redis subscribe stream failed", extra={"err": str(e)})
            finally:
                try:
                    writer.close()
                    await asyncio.wait_for(writer.wait_closed(), timeout=0.5)
                except (TimeoutError, OSError):
                    pass
            await asyncio.sleep(backoff)