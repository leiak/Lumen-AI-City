"""RedisPub RESP 客户端单测 —— 真实 Redis 由 RUN_REDIS_TESTS=1 启用。"""
import asyncio
import os
import socket
import threading

import pytest

from agent_os.redis_pub import RedisPub, RedisStats, parse_redis_addr


def test_parse_redis_addr_default_db():
    assert parse_redis_addr("redis://127.0.0.1:6379") == ("127.0.0.1", 6379, None)
    assert parse_redis_addr("redis://127.0.0.1:6379/2") == ("127.0.0.1", 6379, 2)
    assert parse_redis_addr("redis://:pw@host:1234/0") == ("host", 1234, 0)


def test_parse_redis_addr_invalid():
    with pytest.raises(ValueError):
        parse_redis_addr("not-a-url")
    with pytest.raises(ValueError):
        parse_redis_addr("redis://only-host-no-port")


def test_stats_starts_at_zero():
    pub = RedisPub("redis://127.0.0.1:1/0")
    s = pub.stats()
    assert s == RedisStats()


@pytest.mark.skipif(os.environ.get("RUN_REDIS_TESTS") != "1", reason="real redis required")
def test_publish_roundtrip():
    """真 Redis：起本地 socket-server 模拟 PING + PUBLISH，看 publish 不抛。"""
    server = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    server.bind(("127.0.0.1", 0))
    server.listen(1)
    port = server.getsockname()[1]

    def serve():
        while True:
            try:
                conn, _ = server.accept()
                buf = b""
                while b"\r\n" not in buf:
                    chunk = conn.recv(4096)
                    if not chunk:
                        break
                    buf += chunk
                if b"PING" in buf:
                    conn.sendall(b"+PONG\r\n")
                else:
                    conn.sendall(b":1\r\n")
                conn.close()
            except OSError:
                return

    t = threading.Thread(target=serve, daemon=True)
    t.start()

    async def run():
        pub = RedisPub(f"redis://127.0.0.1:{port}/0")
        await pub.publish("aicity:test", "hello")
        s = pub.stats()
        assert s.messages_published == 1
        assert s.connect_errors == 0

    asyncio.run(run())
    server.close()
