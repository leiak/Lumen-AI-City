"""RedisSub 纯逻辑：SUBSCRIBE 编码 + RESP 推送解析（喂 asyncio.StreamReader）。"""
import asyncio

from agent_os.redis_sub import _encode_subscribe, iter_messages


def _resp_array(*parts: bytes) -> bytes:
    out = b"*%d\r\n" % len(parts)
    for part in parts:
        out += b"$%d\r\n" % len(part) + part + b"\r\n"
    return out


def test_encode_subscribe():
    assert _encode_subscribe("c") == b"*2\r\n$9\r\nSUBSCRIBE\r\n$1\r\nc\r\n"


def _collect(data: bytes) -> list[tuple[str, str]]:
    async def inner() -> list[tuple[str, str]]:
        reader = asyncio.StreamReader(limit=64 * 1024)
        reader.feed_data(data)
        reader.feed_eof()
        return [(ch, pl) async for ch, pl in iter_messages(reader)]

    return asyncio.run(inner())


def test_iter_messages_yields_publish_payload():
    data = (
        _resp_array(b"subscribe", b"chan", b"1")
        + _resp_array(b"message", b"chan", b'{"player_id":"p1"}')
        + _resp_array(b"message", b"other", b"x")
    )
    assert _collect(data) == [
        ("chan", '{"player_id":"p1"}'),
        ("other", "x"),
    ]


def test_iter_messages_skips_confirm_and_unknown():
    data = (
        _resp_array(b"unsubscribe", b"chan", b"0")
        + _resp_array(b"subject", b"chan", b"ignored")
        + _resp_array(b"message", b"chan", b"keep")
    )
    assert _collect(data) == [("chan", "keep")]


def test_iter_messages_returns_on_eof():
    assert _collect(b"") == []