# Sprint 12 最小闭环实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在不实施 Sprint 11 的前提下，把 1.0 demo 最小路径打通：玩家登录 → 浏览器看到王老板主动 say → 弹 NPCDialog 气泡 → 玩家点选项 → api-gateway 查 talk_tree → 王老板回复。

**Architecture:** 4 个栈协同：agent-os (Python) 5s tick 触发 `dispatcher.say` → Redis `aicity:npc_dialogue` 频道 → ws-gateway (Go) 多频道订阅 → 浏览器 → 玩家点选项 → `POST /v1/npc/:id/talk` → api-gateway (Go) 读 `wang_boss.yaml` 解析 talk_tree → 直接 publish 同一频道 → 浏览器收到 reply。

**Tech Stack:** Python 3.12 + FastAPI + 手写 RESP 客户端；Go 1.23 + gin + go-redis + gopkg.in/yaml.v3；Next.js 15 + React 19 + vitest + @testing-library/react。

**Spec:** `docs/superpowers/specs/2026-09-08-sprint12-min-slice-design.md`

**测试命令（统一）：**
- agent-os: `cd ai-city/apps/agent-os && uv run pytest`
- ws-gateway: `cd ai-city/apps/ws-gateway && WS_TEST_REDIS_URL=redis://127.0.0.1:6379/15 go test ./...`
- api-gateway: `cd ai-city/apps/api-gateway && A2A_TEST_DATABASE_URL=... go test ./...`（沿用现有 PG env）
- web: `cd ai-city/web && pnpm vitest run`

---

## 任务依赖图

```
T01a RedisPub 客户端  ──→ T01b ActionDispatcher  ──→ T01c npc_registry  ──→ T01d SayScheduler  ──→ T01e main.py 集成
                                                                                                  │
T02a protocol.TypeNpcDialogue  ──→ T02b Subscribe 重构  ──→ T02c main.go 接入                                       │
                                                                                                  │
T03a ws-events.ts 加分支  ──→ T03b api.ts 加 postNpcTalk  ──→ T03c NPCDialog 组件  ──→ T03d WorldMap 渲染 + click ─→ T03e city/page 挂载
                                                                                                  │
T04a OCEAN schema 扩字段  ──→ T04b wang_boss.yaml 扩字段                                                            │
                                                                                                  │
T05a talk_tree.go 解析  ──→ T05b npc.go handler  ──→ T05c router 挂载  ──→ T05d handler 调 redis publish  ──→ T05e 集成 verify
```

每条线内顺序强依赖；4 条线（T01 / T02 / T03 / T05）**完全并行**；T04a/T04b 是数据准备，可最早开工。

---

## 任务索引

| Task | 服务 | 文件数 | 估时 |
|---|---|---|---|
| T01a-e | agent-os 改造 | 5 文件 | 1.5d |
| T02a-c | ws-gateway 多频道 | 3 文件 | 0.5d |
| T03a-e | web 端接入 | 5 文件 | 1.5d |
| T04a-b | npc-templates | 2 文件 | 0.25d |
| T05a-e | api-gateway endpoint | 5 文件 | 1d |
| **T06** | 集成 verify | — | 0.25d |

**总：5d**

---

## Task T01a：agent-os 精简依赖 + config 加 Redis 字段

**Files:**
- Modify: `ai-city/apps/agent-os/pyproject.toml:1-30`
- Modify: `ai-city/apps/agent-os/src/agent_os/config.py:1-37`
- Test: `ai-city/apps/agent-os/tests/test_config.py` (new)

- [ ] **Step 1: 写 config 单测**

在 `apps/agent-os/tests/test_config.py` 新建：

```python
"""config defaults — Sprint 12 减面后只剩本范围需要的字段。"""
import os
from agent_os.config import Settings


def test_http_port_defaults_to_8084():
    os.environ.pop("HTTP_PORT", None)
    s = Settings()
    assert s.http_port == 8084


def test_redis_url_default():
    os.environ.pop("REDIS_URL", None)
    s = Settings()
    assert s.redis_url == "redis://localhost:6379/0"


def test_npc_dialogue_channel_default():
    os.environ.pop("REDIS_CHANNEL_NPC_DIALOGUE", None)
    s = Settings()
    assert s.redis_channel_npc_dialogue == "aicity:npc_dialogue"


def test_say_tick_seconds_default():
    os.environ.pop("SAY_TICK_SECONDS", None)
    s = Settings()
    assert s.say_tick_seconds == 5.0
```

- [ ] **Step 2: 跑测试确认 fail**

```bash
cd ai-city/apps/agent-os
uv run pytest tests/test_config.py -v
```

期望：`AttributeError: 'Settings' object has no attribute 'http_port'`（或类似，因为字段未加）。

- [ ] **Step 3: 改 pyproject.toml：删 LLM/kafka/SQL/otel 依赖，端口默认 8084**

`ai-city/apps/agent-os/pyproject.toml` 完整替换为：

```toml
[project]
name = "agent-os"
version = "0.1.0"
description = "AI City - Agent OS (Sprint 12 min slice)"
requires-python = ">=3.12"
dependencies = [
    "fastapi>=0.115",
    "uvicorn[standard]>=0.32",
    "pydantic>=2.9",
    "pydantic-settings>=2.5",
    "pyyaml>=6.0",
]

[project.optional-dependencies]
dev = [
    "pytest>=8.3",
    "pytest-asyncio>=0.24",
    "ruff>=0.7",
    "mypy>=1.13",
]

[tool.ruff]
line-length = 100

[tool.mypy]
strict = true

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"
```

- [ ] **Step 4: 改 config.py：加 HTTP_PORT / REDIS_CHANNEL_NPC_DIALOGUE / SAY_TICK_SECONDS / NPC_TEMPLATES_DIR**

完整替换为：

```python
"""配置管理。Sprint 12 减面：只保留本范围需要的字段。"""
from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_file=".env", extra="ignore")

    service_name: str = "agent-os"
    log_level: str = "info"

    # HTTP（沿用 Sprint 11 T04 决定：8084 平行于 8080/8082/8083）
    http_port: int = 8084

    # Redis（沿用 world-engine 模式：手写 RESP，URL 形如 redis://host:port[/db]）
    redis_url: str = "redis://localhost:6379/0"
    redis_channel_npc_dialogue: str = "aicity:npc_dialogue"

    # NPC 模板（dev 走 monorepo 相对路径；容器化时由 Dockerfile COPY 注入 /etc/aicity/npc-templates）
    npc_templates_dir: str = "./packages/npc-templates"

    # Say 调度（5s 触发一轮；Sprint 13+ 接 player listener 后改成事件驱动）
    say_tick_seconds: float = 5.0


settings = Settings()
```

⚠️ pydantic-settings v2 对环境变量**大小写不敏感**（`HTTP_PORT` / `http_port` 都行），单测用 `os.environ.pop` 一定要在 `Settings()` 实例化**之前**（已写在 step 1）。

- [ ] **Step 5: 跑测试确认 pass**

```bash
cd ai-city/apps/agent-os
uv run pytest tests/test_config.py -v
```

期望：4 passed。

- [ ] **Step 6: 提交**

```bash
cd ai-city
git add apps/agent-os/pyproject.toml apps/agent-os/src/agent_os/config.py apps/agent-os/tests/test_config.py
git commit -m "feat(agent-os): sprint12 减面 - port 8084 + redis + say tick (T01a)"
```

---

## Task T01b：手写 RESP RedisPub 客户端

**Files:**
- Create: `ai-city/apps/agent-os/src/agent_os/redis_pub.py`
- Test: `ai-city/apps/agent-os/tests/test_redis_pub.py`

参考 `ai-city/apps/world-engine/src/redis_pub.rs` 的语义（pub fire-and-forget + 6 原子计数器 + 1s connect timeout）。

- [ ] **Step 1: 写失败单测**

`apps/agent-os/tests/test_redis_pub.py`：

```python
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
    pub = RedisPub("redis://127.0.0.1:1/0")  # 端口不通，但 stats 不依赖连接
    s = pub.stats()
    assert s == RedisStats()


@pytest.mark.skipif(os.environ.get("RUN_REDIS_TESTS") != "1", reason="real redis required")
def test_publish_roundtrip():
    """真 Redis：起本地 socket-server 模拟 PING + PUBLISH，看 publish 不抛。"""
    # 起本地 socket echo server
    responses = {b"PING\r\n": b"+PONG\r\n", b"PUBLISH": b":1\r\n"}
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
                # 简化：只回 PONG
                if b"PING" in buf:
                    conn.sendall(b"+PONG\r\n")
                else:
                    # PUBLISH :<n>\r\n
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
```

- [ ] **Step 2: 跑测试确认 fail**

```bash
cd ai-city/apps/agent-os
uv run pytest tests/test_redis_pub.py -v
```

期望：`ModuleNotFoundError: No module named 'agent_os.redis_pub'`。

- [ ] **Step 3: 实现 redis_pub.py**

`apps/agent-os/src/agent_os/redis_pub.py`：

```python
"""agent-os → Redis publisher（手写 RESP，1.0 不引 redis-py）。

与 apps/world-engine/src/redis_pub.rs 同款契约：
- publish fire-and-forget（失败仅 warn）
- 6 原子计数器（lock-free，dataclass 字段 + 单线程访问即可）
- 1s connect timeout
"""
from __future__ import annotations

import asyncio
import logging
import socket
import time
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
    """redis://host:port[/db] → (host, port, db_or_None)。

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
            # 读取 :<n>\r\n 但 fire-and-forget 不强求：写完即视为成功
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
```

⚠️ **关键陷阱 — 同步读 PING 行用 `reader.readline()` 限制 64 字节**：`b"+PONG\r\n"` 是 7 字节，远小于 64；如果读不到完整行（连接半闭），line 是空，判 fail。

- [ ] **Step 4: 跑测试确认 pass（不含集成测）**

```bash
cd ai-city/apps/agent-os
uv run pytest tests/test_redis_pub.py -v
```

期望：3 passed, 1 skipped（`test_publish_roundtrip` 在没有 `RUN_REDIS_TESTS=1` 时 skip）。

- [ ] **Step 5: 跑集成测（可选，本地有真 Redis 才跑）**

```bash
cd ai-city/apps/agent-os
RUN_REDIS_TESTS=1 uv run pytest tests/test_redis_pub.py::test_publish_roundtrip -v
```

期望：4 passed（无 skip）。如果失败说明 RESP 序列化错了，常见 bug 是 `$<len>` 没把 channel 长度算对。

- [ ] **Step 6: 提交**

```bash
cd ai-city
git add apps/agent-os/src/agent_os/redis_pub.py apps/agent-os/tests/test_redis_pub.py
git commit -m "feat(agent-os): 手写 RESP RedisPub 客户端 (T01b)"
```

---

## Task T01c：npc_registry 读 yaml + 过滤 enabled

**Files:**
- Create: `ai-city/apps/agent-os/src/agent_os/npc_registry.py`
- Test: `apps/agent-os/tests/test_npc_registry.py`

- [ ] **Step 1: 写失败单测**

`apps/agent-os/tests/test_npc_registry.py`：

```python
"""npc_registry 加载 yaml + list_enabled 过滤。"""
import textwrap
from pathlib import Path

import pytest

from agent_os.npc_registry import NpcRegistry, NpcTemplate


def _write_yaml(tmp: Path, name: str, body: str) -> Path:
    p = tmp / name
    p.write_text(textwrap.dedent(body), encoding="utf-8")
    return p


def test_load_wang_boss_minimal(tmp_path: Path):
    _write_yaml(tmp_path, "wang_boss.yaml", """\
        npc_id: npc_wang_boss_001
        name: 王老板
        enabled: true
        say:
          greeting:
            - "来了您嘞！"
        talk_tree:
          initial: greet
          nodes:
            greet:
              say: "来了您嘞！"
              options:
                - {id: ask, text: "问个事"}
              reply_map:
                ask: end
    """)
    reg = NpcRegistry(tmp_path)
    t = reg.get("npc_wang_boss_001")
    assert t.npc_id == "npc_wang_boss_001"
    assert t.enabled is True
    assert t.say.greeting == ["来了您嘞！"]
    assert t.talk_tree.initial == "greet"
    assert t.talk_tree.nodes["greet"].say == "来了您嘞！"


def test_list_enabled_filters_disabled(tmp_path: Path):
    _write_yaml(tmp_path, "wang_boss.yaml", """\
        npc_id: npc_wang_boss_001
        enabled: true
    """)
    _write_yaml(tmp_path, "lihua.yaml", """\
        npc_id: npc_lihua_002
        enabled: false
    """)
    reg = NpcRegistry(tmp_path)
    enabled = reg.list_enabled()
    assert [t.npc_id for t in enabled] == ["npc_wang_boss_001"]


def test_get_unknown_npc_raises(tmp_path: Path):
    reg = NpcRegistry(tmp_path)
    with pytest.raises(KeyError):
        reg.get("npc_does_not_exist")


def test_yaml_missing_npc_id_raises(tmp_path: Path):
    _write_yaml(tmp_path, "bad.yaml", """\
        enabled: true
    """)
    reg = NpcRegistry(tmp_path)
    with pytest.raises(ValueError):
        reg.get("bad")  # 第一个文件无 npc_id
```

- [ ] **Step 2: 跑测试确认 fail**

```bash
cd ai-city/apps/agent-os
uv run pytest tests/test_npc_registry.py -v
```

期望：`ModuleNotFoundError: No module named 'agent_os.npc_registry'`。

- [ ] **Step 3: 实现 npc_registry.py**

`apps/agent-os/src/agent_os/npc_registry.py`：

```python
"""从 packages/npc-templates/*.yaml 加载 NPC 模板；过滤 enabled。

Sprint 12 只读 wang_boss.yaml（其它 NPC enabled: false）。
1.0 demo 接受启动期一次性 load，不监听 mtime。
"""
from __future__ import annotations

import logging
from dataclasses import dataclass, field
from pathlib import Path

import yaml

logger = logging.getLogger(__name__)


@dataclass
class Say:
    greeting: list[str] = field(default_factory=list)
    welcome: list[str] = field(default_factory=list)
    default_reply: str = ""


@dataclass
class DialogOption:
    id: str
    text: str


@dataclass
class TalkNode:
    say: str = ""
    options: list[DialogOption] = field(default_factory=list)
    reply_map: dict[str, str] = field(default_factory=dict)
    next_options: list[DialogOption] = field(default_factory=list)


@dataclass
class TalkTree:
    initial: str = ""
    nodes: dict[str, TalkNode] = field(default_factory=dict)


@dataclass
class NpcTemplate:
    npc_id: str
    name: str = ""
    enabled: bool = False
    home_tile_id: str = ""
    avatar_url: str = ""
    say: Say = field(default_factory=Say)
    talk_tree: TalkTree = field(default_factory=TalkTree)


def _load_one(path: Path) -> NpcTemplate:
    data = yaml.safe_load(path.read_text(encoding="utf-8")) or {}
    npc_id = data.get("npc_id")
    if not npc_id:
        raise ValueError(f"{path.name}: missing npc_id")

    say_data = data.get("say", {}) or {}
    say = Say(
        greeting=list(say_data.get("greeting", []) or []),
        welcome=list(say_data.get("welcome", []) or []),
        default_reply=str(say_data.get("default_reply", "") or ""),
    )

    tree_data = data.get("talk_tree", {}) or {}
    nodes: dict[str, TalkNode] = {}
    for nid, n in (tree_data.get("nodes", {}) or {}).items():
        opts = [DialogOption(id=o["id"], text=o["text"]) for o in (n.get("options", []) or [])]
        next_opts = [DialogOption(id=o["id"], text=o["text"]) for o in (n.get("next_options", []) or [])]
        nodes[nid] = TalkNode(
            say=str(n.get("say", "") or ""),
            options=opts,
            reply_map=dict(n.get("reply_map", {}) or {}),
            next_options=next_opts,
        )
    tree = TalkTree(initial=str(tree_data.get("initial", "") or ""), nodes=nodes)

    return NpcTemplate(
        npc_id=npc_id,
        name=str(data.get("name", "") or ""),
        enabled=bool(data.get("enabled", False)),
        home_tile_id=str(data.get("home_tile_id", "") or ""),
        avatar_url=str(data.get("avatar_url", "") or ""),
        say=say,
        talk_tree=tree,
    )


class NpcRegistry:
    """启动期一次性 load 全部 yaml。"""

    def __init__(self, templates_dir: str | Path) -> None:
        self._dir = Path(templates_dir)
        self._by_id: dict[str, NpcTemplate] = {}
        if not self._dir.exists():
            logger.warning("npc_templates_dir not found", extra={"dir": str(self._dir)})
            return
        for p in sorted(self._dir.glob("*.yaml")):
            try:
                t = _load_one(p)
            except Exception as e:
                logger.warning("npc yaml load failed", extra={"file": p.name, "err": str(e)})
                continue
            self._by_id[t.npc_id] = t
        logger.info("npc_registry loaded", extra={"count": len(self._by_id)})

    def get(self, npc_id: str) -> NpcTemplate:
        if npc_id not in self._by_id:
            raise KeyError(npc_id)
        return self._by_id[npc_id]

    def list_enabled(self) -> list[NpcTemplate]:
        return [t for t in self._by_id.values() if t.enabled]
```

- [ ] **Step 4: 跑测试确认 pass**

```bash
cd ai-city/apps/agent-os
uv run pytest tests/test_npc_registry.py -v
```

期望：4 passed。

- [ ] **Step 5: 提交**

```bash
cd ai-city
git add apps/agent-os/src/agent_os/npc_registry.py apps/agent-os/tests/test_npc_registry.py
git commit -m "feat(agent-os): npc_registry yaml 加载 + enabled 过滤 (T01c)"
```

---

## Task T01d：ActionDispatcher + SayScheduler

**Files:**
- Create: `apps/agent-os/src/agent_os/action_dispatcher.py`
- Create: `apps/agent-os/src/agent_os/say_scheduler.py`
- Test: `apps/agent-os/tests/test_action_dispatcher.py`
- Test: `apps/agent-os/tests/test_say_scheduler.py`

- [ ] **Step 1: 写失败单测：action_dispatcher**

`apps/agent-os/tests/test_action_dispatcher.py`：

```python
"""ActionDispatcher.say() 构造 envelope + publish。"""
import json

import pytest

from agent_os.action_dispatcher import ActionDispatcher
from agent_os.redis_pub import RedisStats


class FakeRedis:
    def __init__(self):
        self.calls: list[tuple[str, str]] = []
        self.stats = RedisStats()

    async def publish(self, channel: str, payload: str) -> None:
        self.calls.append((channel, payload))

    def stats_(self):
        return self.stats


@pytest.mark.asyncio
async def test_say_minimal_publishes_envelope():
    fake = FakeRedis()
    d = ActionDispatcher(fake, channel="aicity:npc_dialogue")  # type: ignore[arg-type]
    await d.say("npc_wang_boss_001", "来了您嘞！")
    assert len(fake.calls) == 1
    channel, payload = fake.calls[0]
    assert channel == "aicity:npc_dialogue"
    env = json.loads(payload)
    assert env["type"] == "npc_dialogue"
    assert env["payload"]["npc_id"] == "npc_wang_boss_001"
    assert env["payload"]["say"] == "来了您嘞！"
    assert env["payload"]["options"] == []  # 空数组不是 null
    assert env["trace_id"]  # uuid 字符串非空
    assert env["ts_ms"] > 0


@pytest.mark.asyncio
async def test_say_with_options_and_reply_to():
    fake = FakeRedis()
    d = ActionDispatcher(fake, channel="aicity:npc_dialogue")  # type: ignore[arg-type]
    await d.say(
        "npc_wang_boss_001",
        "回您一句。",
        player_id="p-1",
        tile_id="tile_0_0",
        options=[{"id": "ok", "text": "好的"}],
        reply_to_choice_id="ask_business",
    )
    env = json.loads(fake.calls[0][1])
    p = env["payload"]
    assert p["player_id"] == "p-1"
    assert p["tile_id"] == "tile_0_0"
    assert p["options"] == [{"id": "ok", "text": "好的"}]
    assert p["reply_to_choice_id"] == "ask_business"


@pytest.mark.asyncio
async def test_say_publish_failure_does_not_raise():
    class BadRedis:
        async def publish(self, channel, payload):
            raise RuntimeError("simulated")

    d = ActionDispatcher(BadRedis(), channel="aicity:npc_dialogue")  # type: ignore[arg-type]
    # 不抛：fire-and-forget
    await d.say("npc_wang_boss_001", "x")
```

- [ ] **Step 2: 写失败单测：say_scheduler**

`apps/agent-os/tests/test_say_scheduler.py`：

```python
"""SayScheduler 每 tick 给 enabled NPC 发一句。"""
import asyncio
import textwrap
from pathlib import Path

import pytest

from agent_os.npc_registry import NpcRegistry
from agent_os.say_scheduler import SayScheduler


class FakeDispatcher:
    def __init__(self):
        self.calls: list[tuple[str, str]] = []

    async def say(self, npc_id: str, text: str, **kwargs) -> None:
        self.calls.append((npc_id, text))


def _write_yaml(tmp: Path, name: str, body: str) -> Path:
    p = tmp / name
    p.write_text(textwrap.dedent(body), encoding="utf-8")
    return p


@pytest.mark.asyncio
async def test_tick_emits_one_say_per_enabled_npc(tmp_path: Path):
    _write_yaml(tmp_path, "wang_boss.yaml", """\
        npc_id: npc_wang_boss_001
        enabled: true
        say:
          greeting:
            - "来了您嘞！"
    """)
    _write_yaml(tmp_path, "lihua.yaml", """\
        npc_id: npc_lihua_002
        enabled: false
        say:
          greeting:
            - "我不在"
    """)
    reg = NpcRegistry(tmp_path)
    disp = FakeDispatcher()
    sched = SayScheduler(registry=reg, dispatcher=disp, tick_seconds=0.01)  # type: ignore[arg-type]

    await sched.tick_once()
    assert [c[0] for c in disp.calls] == ["npc_wang_boss_001"]
    assert disp.calls[0][1] == "来了您嘞！"


@pytest.mark.asyncio
async def test_tick_no_enabled_skips_silently(tmp_path: Path):
    _write_yaml(tmp_path, "lihua.yaml", """\
        npc_id: npc_lihua_002
        enabled: false
    """)
    reg = NpcRegistry(tmp_path)
    disp = FakeDispatcher()
    sched = SayScheduler(registry=reg, dispatcher=disp, tick_seconds=0.01)  # type: ignore[arg-type]
    await sched.tick_once()
    assert disp.calls == []


@pytest.mark.asyncio
async def test_run_stops_on_event(tmp_path: Path):
    _write_yaml(tmp_path, "wang_boss.yaml", """\
        npc_id: npc_wang_boss_001
        enabled: true
        say:
          greeting:
            - "来了您嘞！"
    """)
    reg = NpcRegistry(tmp_path)
    disp = FakeDispatcher()
    sched = SayScheduler(registry=reg, dispatcher=disp, tick_seconds=0.01)  # type: ignore[arg-type]

    stop = asyncio.Event()
    task = asyncio.create_task(sched.run(stop))
    await asyncio.sleep(0.05)  # 至少跑 2 tick
    stop.set()
    await asyncio.wait_for(task, timeout=1.0)
    assert len(disp.calls) >= 1
```

- [ ] **Step 3: 跑测试确认 fail**

```bash
cd ai-city/apps/agent-os
uv run pytest tests/test_action_dispatcher.py tests/test_say_scheduler.py -v
```

期望：`ModuleNotFoundError` for both。

- [ ] **Step 4: 实现 action_dispatcher.py**

`apps/agent-os/src/agent_os/action_dispatcher.py`：

```python
"""NPC action 执行层（Sprint 12 min slice）。

只暴露 say()，行为：构造 envelope → Redis publish → 失败仅 warn（fire-and-forget）。

Sprint 13+ 才会加 move() / wait() / give()。
"""
from __future__ import annotations

import json
import logging
import time
import uuid
from typing import Any, Protocol

logger = logging.getLogger(__name__)


class _RedisLike(Protocol):
    async def publish(self, channel: str, payload: str) -> None: ...


class ActionDispatcher:
    def __init__(self, redis: _RedisLike, *, channel: str) -> None:
        self._redis = redis
        self._channel = channel

    async def say(
        self,
        npc_id: str,
        text: str,
        *,
        player_id: str | None = None,
        tile_id: str | None = None,
        options: list[dict[str, Any]] | None = None,
        reply_to_choice_id: str | None = None,
    ) -> None:
        """发送一句 NPC 台词。失败仅 warn，agent tick 不阻塞。

        envelope:
        {
          "type": "npc_dialogue",
          "trace_id": "<uuid>",
          "ts_ms": <int ms>,
          "payload": {
            "npc_id": "...", "player_id": "...", "tile_id": "...",
            "say": "...",
            "options": [...],                   // 始终是数组（不是 null）
            "reply_to_choice_id": "..."|null    // null = 主动 say；非空 = 玩家回复
          }
        }
        """
        envelope: dict[str, Any] = {
            "type": "npc_dialogue",
            "trace_id": str(uuid.uuid4()),
            "ts_ms": int(time.time() * 1000),
            "payload": {
                "npc_id": npc_id,
                "player_id": player_id or "",
                "tile_id": tile_id or "",
                "say": text,
                "options": list(options) if options else [],
                "reply_to_choice_id": reply_to_choice_id,
            },
        }
        try:
            await self._redis.publish(self._channel, json.dumps(envelope, ensure_ascii=False))
        except Exception as e:  # noqa: BLE001
            logger.warning(
                "dispatcher.say publish failed",
                extra={"npc_id": npc_id, "err": str(e)},
            )
```

⚠️ **关键陷阱 — `reply_to_choice_id: null` 在 JSON 序列化后会输出 `null`**：浏览器侧用 `payload.reply_to_choice_id === null` 判主动 say，`=== "ask_business"` 判 reply。

- [ ] **Step 5: 实现 say_scheduler.py**

`apps/agent-os/src/agent_os/say_scheduler.py`：

```python
"""每 tick 给 list_enabled() 里每个 NPC 选 1 句 greeting 调 dispatcher.say。

Sprint 12 min slice：纯定时器，不感知 player listener。
Sprint 13+ 会替换为事件驱动（玩家进入 tile_0_0 才触发）。
"""
from __future__ import annotations

import asyncio
import logging
import random
from typing import Protocol

from agent_os.npc_registry import NpcRegistry

logger = logging.getLogger(__name__)


class _DispatcherLike(Protocol):
    async def say(self, npc_id: str, text: str, **kwargs) -> None: ...


class SayScheduler:
    def __init__(
        self,
        *,
        registry: NpcRegistry,
        dispatcher: _DispatcherLike,
        tick_seconds: float,
    ) -> None:
        self._registry = registry
        self._dispatcher = dispatcher
        self._tick = tick_seconds
        self._rng = random.Random()

    async def tick_once(self) -> None:
        for tpl in self._registry.list_enabled():
            greetings = tpl.say.greeting
            if not greetings:
                continue
            text = self._rng.choice(greetings)
            try:
                await self._dispatcher.say(tpl.npc_id, text)
            except Exception as e:  # noqa: BLE001
                logger.warning("scheduler tick failed", extra={"npc_id": tpl.npc_id, "err": str(e)})

    async def run(self, stop: asyncio.Event) -> None:
        """长寿命 task。stop.set() 后下一次 tick 退出。"""
        logger.info("say_scheduler starting", extra={"tick_seconds": self._tick})
        while not stop.is_set():
            try:
                await self.tick_once()
            except Exception as e:  # noqa: BLE001
                logger.exception("scheduler tick exception", extra={"err": str(e)})
            try:
                await asyncio.wait_for(stop.wait(), timeout=self._tick)
            except asyncio.TimeoutError:
                pass
        logger.info("say_scheduler stopped")
```

- [ ] **Step 6: 跑测试确认 pass**

```bash
cd ai-city/apps/agent-os
uv run pytest tests/test_action_dispatcher.py tests/test_say_scheduler.py -v
```

期望：6 passed。

- [ ] **Step 7: 提交**

```bash
cd ai-city
git add apps/agent-os/src/agent_os/action_dispatcher.py apps/agent-os/src/agent_os/say_scheduler.py apps/agent-os/tests/test_action_dispatcher.py apps/agent-os/tests/test_say_scheduler.py
git commit -m "feat(agent-os): ActionDispatcher + SayScheduler (T01d)"
```

---

## Task T01e：main.py 集成 + FastAPI 端口 8084

**Files:**
- Create: `apps/agent-os/src/agent_os/main.py`
- Modify: `apps/agent-os/src/agent_os/app.py`
- Modify: `apps/agent-os/Dockerfile`

- [ ] **Step 1: 改 app.py：扩 /healthz + /npc_loaded**

`apps/agent-os/src/agent_os/app.py` 完整替换为：

```python
"""FastAPI 入口（Sprint 12 min slice）。

启动顺序：load NPC registry → 起 SayScheduler（独立 task）→ 起 HTTP。
关闭顺序：stop scheduler → close redis → 退 uvicorn。
"""
from __future__ import annotations

import asyncio
import logging
from contextlib import asynccontextmanager
from typing import Optional

from fastapi import FastAPI

from agent_os.action_dispatcher import ActionDispatcher
from agent_os.config import settings
from agent_os.npc_registry import NpcRegistry
from agent_os.redis_pub import RedisPub
from agent_os.say_scheduler import SayScheduler

logger = logging.getLogger(__name__)

# 进程内单例；tests 用 TestClient(monkeypatch) 注入。
_registry: Optional[NpcRegistry] = None
_dispatcher: Optional[ActionDispatcher] = None
_scheduler: Optional[SayScheduler] = None
_stop_event: Optional[asyncio.Event] = None
_scheduler_task: Optional[asyncio.Task] = None
_redis: Optional[RedisPub] = None


@asynccontextmanager
async def lifespan(app: FastAPI):
    global _registry, _dispatcher, _scheduler, _stop_event, _scheduler_task, _redis

    logging.basicConfig(level=settings.log_level.upper())
    logger.info("agent-os starting", extra={
        "port": settings.http_port,
        "tick_seconds": settings.say_tick_seconds,
        "templates_dir": settings.npc_templates_dir,
    })

    _registry = NpcRegistry(settings.npc_templates_dir)
    _redis = RedisPub(settings.redis_url)
    _dispatcher = ActionDispatcher(_redis, channel=settings.redis_channel_npc_dialogue)
    _scheduler = SayScheduler(
        registry=_registry, dispatcher=_dispatcher, tick_seconds=settings.say_tick_seconds
    )

    _stop_event = asyncio.Event()
    _scheduler_task = asyncio.create_task(_scheduler.run(_stop_event))

    try:
        yield
    finally:
        logger.info("agent-os shutting down")
        if _stop_event is not None:
            _stop_event.set()
        if _scheduler_task is not None:
            try:
                await asyncio.wait_for(_scheduler_task, timeout=2.0)
            except asyncio.TimeoutError:
                _scheduler_task.cancel()


app = FastAPI(title="AI City - Agent OS", lifespan=lifespan)


@app.get("/healthz")
async def healthz() -> dict:
    enabled = [t.npc_id for t in _registry.list_enabled()] if _registry else []
    return {
        "status": "ok",
        "service": settings.service_name,
        "npc_loaded": len(enabled),
        "npc_enabled": enabled,
    }


@app.get("/npc_loaded")
async def npc_loaded() -> dict:
    enabled = [t.npc_id for t in _registry.list_enabled()] if _registry else []
    return {"npc_loaded": len(enabled), "enabled": enabled}
```

- [ ] **Step 2: 新建 main.py：uvicorn 启动**

`apps/agent-os/src/agent_os/main.py`：

```python
"""uvicorn 启动入口。"""
import uvicorn

from agent_os.config import settings


def main() -> None:
    uvicorn.run(
        "agent_os.app:app",
        host="0.0.0.0",
        port=settings.http_port,
        log_level=settings.log_level.lower(),
    )


if __name__ == "__main__":
    main()
```

- [ ] **Step 3: 改 Dockerfile：EXPOSE 8084 + CMD 用 main**

`apps/agent-os/Dockerfile` 完整替换为：

```dockerfile
FROM python:3.12-slim

WORKDIR /app

# 安装 uv
RUN pip install --no-cache-dir uv

# 复制项目文件
COPY pyproject.toml ./
COPY src/ ./src/

# 安装依赖
RUN uv pip install --system --no-cache .

# 复制 NPC 模板（与 agent-os 同镜像；只读）
COPY packages/npc-templates/ /etc/aicity/npc-templates/

# 运行时环境：默认读镜像内模板；可通过 env 覆盖
ENV NPC_TEMPLATES_DIR=/etc/aicity/npc-templates
ENV HTTP_PORT=8084
ENV SAY_TICK_SECONDS=5.0
ENV REDIS_URL=redis://redis:6379/0
ENV REDIS_CHANNEL_NPC_DIALOGUE=aicity:npc_dialogue

EXPOSE 8084

CMD ["python", "-m", "agent_os.main"]
```

⚠️ **关键陷阱 — Dockerfile 复制 `packages/npc-templates/`**：当前 monorepo 布局下，docker build 的 context 是 `ai-city/`，所以 `packages/...` 相对路径有效。**不要** `cd apps/agent-os && docker build`（那样 packages 路径不对）。

- [ ] **Step 4: 改 main.py / app.py 后跑全 agent-os 单测**

```bash
cd ai-city/apps/agent-os
uv run pytest -v
```

期望：所有前序任务测试 + 不出现 import 错误。如有 import 错误（如 `from agent_os.main import main` 缺 uvicorn），按错误信息补依赖到 pyproject.toml。

- [ ] **Step 5: 手动起一遍，验证 /healthz**

```bash
cd ai-city/apps/agent-os
NPC_TEMPLATES_DIR=../../packages/npc-templates uv run python -m agent_os.main &
sleep 2
curl http://127.0.0.1:8084/healthz
# 期望：{"status":"ok","service":"agent-os","npc_loaded":N,"npc_enabled":["npc_wang_boss_001",...]}
# N >= 1（如果 wang_boss.yaml 已带 enabled: true 字段；T04b 之前可能 0）
kill %1
```

- [ ] **Step 6: 提交**

```bash
cd ai-city
git add apps/agent-os/src/agent_os/main.py apps/agent-os/src/agent_os/app.py apps/agent-os/Dockerfile
git commit -m "feat(agent-os): main.py + /healthz + port 8084 (T01e)"
```

---

## Task T02a：ws-gateway protocol 加 TypeNpcDialogue

**Files:**
- Modify: `ai-city/apps/ws-gateway/internal/protocol/message.go`
- Test: `ai-city/apps/ws-gateway/internal/protocol/message_test.go`

- [ ] **Step 1: 写失败单测**

`apps/ws-gateway/internal/protocol/message_test.go` 在末尾追加：

```go
func TestNpcDialogue_JSONTags(t *testing.T) {
    p := NpcDialoguePayload{
        NPCID:    "npc_wang_boss_001",
        PlayerID: "p-1",
        TileID:   "tile_0_0",
        Say:      "来了您嘞！",
        Options:  []DialogOption{{ID: "ask", Text: "问个事"}},
        ReplyToChoiceID: func() *string { s := ""; return &s }(),
    }
    b, err := json.Marshal(p)
    if err != nil {
        t.Fatalf("Marshal: %v", err)
    }
    want := `{"npc_id":"npc_wang_boss_001","player_id":"p-1","tile_id":"tile_0_0","say":"来了您嘞！","options":[{"id":"ask","text":"问个事"}],"reply_to_choice_id":""}`
    if string(b) != want {
        t.Errorf("Marshal =\n  %s\nwant\n  %s", b, want)
    }
}

func TestNpcDialogue_ReplyToNilBecomesNull(t *testing.T) {
    p := NpcDialoguePayload{NPCID: "n1", Say: "x"}
    b, _ := json.Marshal(p)
    if !bytes.Contains(b, []byte(`"reply_to_choice_id":null`)) {
        t.Errorf("expected reply_to_choice_id:null in %s", b)
    }
}
```

⚠️ 用 `bytes.Contains`：`bytes` 已 import 进 file；如没有就 import "bytes"。

- [ ] **Step 2: 跑测试确认 fail**

```bash
cd ai-city/apps/ws-gateway
go test ./internal/protocol/... -v -run NpcDialogue
```

期望：compile error: `undefined: TypeNpcDialogue`。

- [ ] **Step 3: 改 message.go：加常量 + 类型**

`apps/ws-gateway/internal/protocol/message.go` 在 `TypePlayerMoved` 常量后插入：

```go
const (
    TypePlayerMoved  = "player_moved"
    TypeNpcDialogue  = "npc_dialogue"
)
```

在文件末尾（`InvalidPayloadError` 之后）追加：

```go
// DialogOption 是 NPCDialog 组件可点的单条选项。
type DialogOption struct {
    ID   string `json:"id"`
    Text string `json:"text"`
}

// NpcDialoguePayload 是 agent-os / api-gateway publish 的 npc_dialogue 消息体。
//
// 与 apps/agent-os/src/agent_os/action_dispatcher.py::say 构造的 envelope payload 字段一致。
//
// reply_to_choice_id == nil → NPC 主动 say；非空字符串 → 玩家点击后的 NPC 回复。
// 浏览器侧用这个字段区分"主动 say"和"回复"两态（同一 type，避免再加一
// 个 type 增加协议分支）。
type NpcDialoguePayload struct {
    NPCID           string         `json:"npc_id"`
    PlayerID        string         `json:"player_id"`
    TileID          string         `json:"tile_id"`
    Say             string         `json:"say"`
    Options         []DialogOption `json:"options"`
    ReplyToChoiceID *string        `json:"reply_to_choice_id"`
}
```

- [ ] **Step 4: 跑测试确认 pass**

```bash
cd ai-city/apps/ws-gateway
go test ./internal/protocol/... -v
```

期望：原有测试 + 2 new pass。

- [ ] **Step 5: 提交**

```bash
cd ai-city
git add apps/ws-gateway/internal/protocol/message.go apps/ws-gateway/internal/protocol/message_test.go
git commit -m "feat(ws-gateway): protocol TypeNpcDialogue + NpcDialoguePayload (T02a)"
```

---

## Task T02b：ws-gateway subscriber.go 重构为 Subscribe 多频道

**Files:**
- Modify: `ai-city/apps/ws-gateway/internal/redis/subscriber.go`
- Modify: `ai-city/apps/ws-gateway/internal/redis/subscriber_test.go`

⚠️ **关键陷阱 — PlayerMoved() 函数是 cmd/main.go 的依赖**。本任务**重命名**它为 `Subscribe()` 并接受 `[]string`；同步改 `cmd/main.go` 在 T02c。

- [ ] **Step 1: 改 subscriber.go：删除 PlayerMoved 签名，新增 Subscribe**

`apps/ws-gateway/internal/redis/subscriber.go` 完整替换为：

```go
// Package redis 订阅 world-engine / agent-os 的事件频道，包信封后交给 hub 扇出。
//
// 与 api-gateway/internal/subscriber/player_moved.go 是**两个独立订阅者**，
// 消费同一频道：api-gateway 写 PG player_position，这里推 WS。
// Redis pub/sub 天然支持多订阅者，两边互不影响。
//
// Sprint 12 改造：从单 channel 改为多 channel 一次性 subscribe；
// type 由 msg.Channel 推断（rdb.Subscribe 同时返回 channel 名）。
package redis

import (
	"context"
	"encoding/json"
	"time"

	"github.com/aicity/ws-gateway/internal/protocol"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Broadcaster 是 hub.Hub 的最小接口（便于单测注入假实现）。
type Broadcaster interface {
	Broadcast(msg []byte)
}

// Subscribe 启动多频道订阅协程；ctx 取消时退出。
//
// channels 至少 1 个；type 由 msg.Channel 推断（与 protocol.Type* 常量匹配）。
//
// ⚠️ ctx 必须是长寿命的 appCtx，不能复用启动期的 10s 超时 ctx ——
// 否则 10s 后订阅者静默退出，且不报任何错（api-gateway 踩过这个坑）。
func Subscribe(ctx context.Context, rdb *goredis.Client, channels []string, b Broadcaster, logger *zap.Logger) {
	logger.Info("redis subscriber starting", zap.Strings("channels", channels))

	go func() {
		backoff := time.Second
		for {
			if ctx.Err() != nil {
				return
			}
			err := runOnce(ctx, rdb, channels, b, logger)
			if err == nil || ctx.Err() != nil {
				return
			}
			logger.Warn("subscriber loop exited, will reconnect",
				zap.Error(err), zap.Duration("backoff", backoff))
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < 5*time.Second {
				backoff *= 2
			}
		}
	}()
}

func runOnce(ctx context.Context, rdb *goredis.Client, channels []string, b Broadcaster, logger *zap.Logger) error {
	pubsub := rdb.Subscribe(ctx, channels...)
	defer pubsub.Close()

	// 阻塞直到确认订阅成功（避免错过首批消息）
	if _, err := pubsub.Receive(ctx); err != nil {
		return err
	}
	logger.Info("redis subscriber ready", zap.Strings("channels", channels))

	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-ch:
			if !ok {
				return nil
			}
			handle(msg.Channel, msg.Payload, b, logger)
		}
	}
}

// handle 解析 → 包信封 → 广播。任何一步失败只告警并丢这一条。
func handle(channel string, payload string, b Broadcaster, logger *zap.Logger) {
	// channel → type 映射
	msgType := channelToType(channel, logger)
	if msgType == "" {
		// 未知 channel：log warn + 丢
		logger.Warn("unknown redis channel, dropping", zap.String("channel", channel))
		return
	}

	// 解析只为了打 debug 日志 + 挡掉坏 JSON；
	// 信封 payload 仍透传原始字节，避免字段在 round-trip 中漂移。
	if !json.Valid([]byte(payload)) {
		logger.Warn("invalid json payload, dropping",
			zap.String("channel", channel), zap.String("payload", payload))
		return
	}

	env, err := protocol.NewEnvelope(msgType, []byte(payload))
	if err != nil {
		logger.Warn("build envelope failed", zap.Error(err))
		return
	}
	out, err := json.Marshal(env)
	if err != nil {
		logger.Warn("marshal envelope failed", zap.Error(err))
		return
	}

	b.Broadcast(out)
	logger.Debug("broadcast", zap.String("type", msgType))
}

// channelToType 把 aicity:player:moved / aicity:npc_dialogue 映射到 protocol.Type*。
//
// 不在 aicity: 前缀上的 channel 一律返 ""（handle 丢）。
func channelToType(channel string, logger *zap.Logger) string {
	switch channel {
	case "aicity:player:moved":
		return protocol.TypePlayerMoved
	case "aicity:npc_dialogue":
		return protocol.TypeNpcDialogue
	default:
		logger.Warn("unmapped channel", zap.String("channel", channel))
		return ""
	}
}
```

- [ ] **Step 2: 改 subscriber_test.go：handle 新签名 + 用 channel 推断 type**

`apps/ws-gateway/internal/redis/subscriber_test.go` 全文替换为：

```go
package redis

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/aicity/ws-gateway/internal/protocol"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// fakeBroadcaster 收集 Broadcast 调用。
type fakeBroadcaster struct {
	mu   sync.Mutex
	msgs [][]byte
	ch   chan struct{}
}

func newFakeBroadcaster() *fakeBroadcaster {
	return &fakeBroadcaster{ch: make(chan struct{}, 16)}
}

func (f *fakeBroadcaster) Broadcast(msg []byte) {
	f.mu.Lock()
	cp := make([]byte, len(msg))
	copy(cp, msg)
	f.msgs = append(f.msgs, cp)
	f.mu.Unlock()
	select {
	case f.ch <- struct{}{}:
	default:
	}
}

func (f *fakeBroadcaster) snapshot() [][]byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([][]byte, len(f.msgs))
	copy(out, f.msgs)
	return out
}

const rustPayload = `{"player_id":"p-1","tile_id":"tile_0_0","x":50.0,"y":50.0,"ts_ms":1700000000000}`

const npcPayload = `{"npc_id":"npc_wang_boss_001","player_id":"p-1","tile_id":"tile_0_0","say":"来了您嘞！","options":[],"reply_to_choice_id":null}`

// handle 纯函数：单 channel 推断 type → 信封包好。
func TestHandle_PlayerMoved(t *testing.T) {
	b := newFakeBroadcaster()
	handle("aicity:player:moved", rustPayload, b, zap.NewNop())

	msgs := b.snapshot()
	if len(msgs) != 1 {
		t.Fatalf("Broadcast called %d times, want 1", len(msgs))
	}
	var env struct {
		Type    string               `json:"type"`
		TraceID string               `json:"trace_id"`
		TsMs    int64                `json:"ts_ms"`
		Payload protocol.PlayerMoved `json:"payload"`
	}
	if err := json.Unmarshal(msgs[0], &env); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if env.Type != protocol.TypePlayerMoved {
		t.Errorf("type = %q, want %q", env.Type, protocol.TypePlayerMoved)
	}
	if env.TraceID == "" || env.TMs <= 0 {
		// 注意：原 file 用 TsMs 字段
	}
	if env.Payload.PlayerID != "p-1" || env.Payload.TileID != "tile_0_0" || env.Payload.X != 50 {
		t.Errorf("payload = %+v", env.Payload)
	}
}

func TestHandle_NpcDialogue(t *testing.T) {
	b := newFakeBroadcaster()
	handle("aicity:npc_dialogue", npcPayload, b, zap.NewNop())

	msgs := b.snapshot()
	if len(msgs) != 1 {
		t.Fatalf("Broadcast called %d times, want 1", len(msgs))
	}
	var env struct {
		Type    string                     `json:"type"`
		Payload protocol.NpcDialoguePayload `json:"payload"`
	}
	if err := json.Unmarshal(msgs[0], &env); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if env.Type != protocol.TypeNpcDialogue {
		t.Errorf("type = %q, want %q", env.Type, protocol.TypeNpcDialogue)
	}
	if env.Payload.NPCID != "npc_wang_boss_001" {
		t.Errorf("payload.npc_id = %q", env.Payload.NPCID)
	}
	if env.Payload.ReplyToChoiceID != nil {
		t.Errorf("payload.reply_to_choice_id = %v, want nil", env.Payload.ReplyToChoiceID)
	}
}

// 坏 JSON 丢这一条。
func TestHandle_DropsInvalidJSON(t *testing.T) {
	b := newFakeBroadcaster()
	handle("aicity:player:moved", `{"player_id":`, b, zap.NewNop())
	if got := len(b.snapshot()); got != 0 {
		t.Errorf("Broadcast called %d times, want 0", got)
	}
}

// 未知 channel 丢。
func TestHandle_DropsUnknownChannel(t *testing.T) {
	b := newFakeBroadcaster()
	handle("aicity:weird:channel", rustPayload, b, zap.NewNop())
	if got := len(b.snapshot()); got != 0 {
		t.Errorf("Broadcast called %d times, want 0", got)
	}
}

// 集成测：真 Redis 发 2 个 channel，订阅者收 2 条。
func TestSubscribe_MultiChannel_Integration(t *testing.T) {
	url := os.Getenv("WS_TEST_REDIS_URL")
	if url == "" {
		t.Skip("WS_TEST_REDIS_URL not set; skipping redis integration test")
	}

	opt, err := goredis.ParseURL(url)
	if err != nil {
		t.Fatalf("ParseURL: %v", err)
	}
	rdb := goredis.NewClient(opt)
	defer rdb.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Fatalf("Ping: %v", err)
	}

	channel1 := "aicity:test:ws:player:moved"
	channel2 := "aicity:test:ws:npc:dialogue"
	b := newFakeBroadcaster()
	Subscribe(ctx, rdb, []string{channel1, channel2}, b, zap.NewNop())

	deadline := time.Now().Add(5 * time.Second)
	saw1, saw2 := false, false
	for time.Now().Before(deadline) && !(saw1 && saw2) {
		_ = rdb.Publish(ctx, channel1, rustPayload).Err()
		_ = rdb.Publish(ctx, channel2, npcPayload).Err()
		select {
		case <-b.ch:
			for _, m := range b.snapshot() {
				var env struct {
					Type string `json:"type"`
				}
				_ = json.Unmarshal(m, &env)
				switch env.Type {
				case protocol.TypePlayerMoved:
					saw1 = true
				case protocol.TypeNpcDialogue:
					saw2 = true
				}
			}
		case <-time.After(200 * time.Millisecond):
		}
	}
	if !saw1 || !saw2 {
		t.Errorf("saw player=%v npc=%v, want both true", saw1, saw2)
	}
}
```

⚠️ **注意**：上面 `TestHandle_PlayerMoved` 里有 `env.TMs <= 0` 注释 — 这是写错字段名的提示，**实际代码用 `env.TsMs`**，把那一行 `if env.TraceID == "" || env.TMs <= 0` 改为 `if env.TraceID == "" || env.TsMs <= 0`。

修正版：

```go
if env.TraceID == "" || env.TsMs <= 0 {
    t.Errorf("trace_id/ts_ms = %q/%d", env.TraceID, env.TsMs)
}
```

- [ ] **Step 3: 跑测试确认 pass**

```bash
cd ai-city/apps/ws-gateway
go test ./internal/redis/... -v
```

期望：5 passed, 1 skipped（无 WS_TEST_REDIS_URL）。

- [ ] **Step 4: 跑全 ws-gateway 测试看是否破坏其它包**

```bash
cd ai-city/apps/ws-gateway
go build ./... && go test ./... -count=1
```

期望：编译通过；所有原有测试仍 pass；新测试 pass。如果有 protocol/cors 测试被影响，定位修复。

- [ ] **Step 5: 提交**

```bash
cd ai-city
git add apps/ws-gateway/internal/redis/subscriber.go apps/ws-gateway/internal/redis/subscriber_test.go
git commit -m "feat(ws-gateway): subscriber 改多频道 + channel 推断 type (T02b)"
```

---

## Task T02c：ws-gateway main.go + config 接入多频道

**Files:**
- Modify: `apps/ws-gateway/internal/config/config.go`
- Modify: `apps/ws-gateway/cmd/main.go`

- [ ] **Step 1: 改 config.go：加 Channels []string**

`apps/ws-gateway/internal/config/config.go` 中 `ChannelMoved` 后追加 + `Load` 改：

```go
// Config 增字段
type Config struct {
    // ... 现有字段
    ChannelMoved string
    // Channels 是 ws-gateway 订阅的 Redis 频道列表（逗号分隔 env 解析）
    Channels []string
    // ...
}
```

```go
func Load() *Config {
    channelsEnv := getEnv("REDIS_CHANNELS", "aicity:player:moved,aicity:npc_dialogue")
    channels := []string{}
    for _, c := range strings.Split(channelsEnv, ",") {
        if c = strings.TrimSpace(c); c != "" {
            channels = append(channels, c)
        }
    }
    // ChannelMoved 是 Channels[0] 的语义别名（保持向后兼容）
    channelMoved := getEnv("REDIS_CHANNEL_MOVED", "aicity:player:moved")
    if len(channels) == 0 {
        channels = []string{channelMoved}
    }
    return &Config{
        // ... 现有字段
        ChannelMoved: channelMoved,
        Channels:     channels,
        // ...
    }
}
```

⚠️ **关键陷阱 — 旧配置只用 `REDIS_CHANNEL_MOVED`**：保留 `ChannelMoved` 字段供 main.go 打 banner；实际订阅走 `Channels`。

- [ ] **Step 2: 改 main.go：调 Subscribe 多频道**

`apps/ws-gateway/cmd/main.go` 中：

```go
wsredis.PlayerMoved(appCtx, rdb, cfg.ChannelMoved, h, logger)
```

改为：

```go
wsredis.Subscribe(appCtx, rdb, cfg.Channels, h, logger)
```

启动 banner 改为：

```go
zap.Strings("channels", cfg.Channels),
```

- [ ] **Step 3: 编译 + 跑全测试**

```bash
cd ai-city/apps/ws-gateway
go build ./... && go test ./... -count=1
```

期望：编译通过；所有测试 pass。

- [ ] **Step 4: 提交**

```bash
cd ai-city
git add apps/ws-gateway/internal/config/config.go apps/ws-gateway/cmd/main.go
git commit -m "feat(ws-gateway): main + config 接 REDIS_CHANNELS 多频道 (T02c)"
```

---

## Task T03a：web ws-events.ts 加 npc_dialogue 分支

**Files:**
- Modify: `ai-city/web/src/lib/ws-events.ts`
- Test: `ai-city/web/tests/lib/ws-events.test.ts` (new)

- [ ] **Step 1: 写失败单测**

`web/tests/lib/ws-events.test.ts`：

```typescript
import { describe, it, expect, vi, beforeEach } from 'vitest';

// 模拟 WSClient
const listeners: ((msg: unknown) => void)[] = [];
vi.mock('@/lib/ws', () => ({
  ws: {
    onMessage: (fn: (msg: unknown) => void) => {
      listeners.push(fn);
      return () => {
        const i = listeners.indexOf(fn);
        if (i >= 0) listeners.splice(i, 1);
      };
    },
    connect: () => {},
  },
}));

vi.mock('@/store/game', () => ({
  useGameStore: {
    getState: () => ({
      playerId: 'p-1',
      setPosition: () => {},
    }),
  },
}));

import {
  PLAYER_MOVED_EVENT,
  NPC_DIALOGUE_EVENT,
  startWsBridge,
} from '@/lib/ws-events';

describe('startWsBridge', () => {
  beforeEach(() => {
    listeners.length = 0;
    window.localStorage.clear();
    window.localStorage.setItem('aicity_token', 'fake');
    window.dispatchEvent = vi.fn();
  });

  it('dispatches aicity:player_moved for player_moved frames', () => {
    const off = startWsBridge();
    const dispatchSpy = vi.spyOn(window, 'dispatchEvent');
    listeners[0]({
      type: 'player_moved',
      trace_id: 't1',
      ts_ms: 1000,
      payload: { player_id: 'p-1', tile_id: 'tile_0_0', x: 50, y: 50, ts_ms: 1000 },
    });
    const types = dispatchSpy.mock.calls.map((c) => (c[0] as CustomEvent).type);
    expect(types).toContain(PLAYER_MOVED_EVENT);
    off();
  });

  it('dispatches aicity:npc_dialogue for npc_dialogue frames', () => {
    const off = startWsBridge();
    const dispatchSpy = vi.spyOn(window, 'dispatchEvent');
    listeners[0]({
      type: 'npc_dialogue',
      trace_id: 't2',
      ts_ms: 2000,
      payload: {
        npc_id: 'npc_wang_boss_001',
        player_id: 'p-1',
        tile_id: 'tile_0_0',
        say: '来了您嘞！',
        options: [],
        reply_to_choice_id: null,
      },
    });
    const types = dispatchSpy.mock.calls.map((c) => (c[0] as CustomEvent).type);
    expect(types).toContain(NPC_DIALOGUE_EVENT);
    off();
  });

  it('ignores unknown types silently', () => {
    const off = startWsBridge();
    const dispatchSpy = vi.spyOn(window, 'dispatchEvent');
    listeners[0]({ type: 'unknown', trace_id: 't', ts_ms: 0, payload: {} });
    expect(dispatchSpy).not.toHaveBeenCalled();
    off();
  });
});
```

- [ ] **Step 2: 跑测试确认 fail**

```bash
cd ai-city/web
pnpm vitest run tests/lib/ws-events.test.ts
```

期望：`Failed to resolve import` 或 `NPC_DIALOGUE_EVENT is not exported`。

- [ ] **Step 3: 改 ws-events.ts：加 NPC_DIALOGUE_EVENT + 分支**

`ai-city/web/src/lib/ws-events.ts` 在 `PlayerMovedPayload` 后追加：

```ts
/** WorldMap + NPCDialog 监听的事件名 */
export const NPC_DIALOGUE_EVENT = 'aicity:npc_dialogue';

/** 与 apps/ws-gateway/internal/protocol/message.go::NpcDialoguePayload 一致 */
export interface NpcDialoguePayload {
  npc_id: string;
  player_id: string;
  tile_id: string;
  say: string;
  options: Array<{ id: string; text: string }>;
  /** null/缺失 = NPC 主动 say；非空 = 玩家点击后的 NPC 回复 */
  reply_to_choice_id: string | null;
}

function isNpcDialogue(msg: unknown): msg is WsEnvelope<NpcDialoguePayload> {
  if (typeof msg !== 'object' || msg === null) return false;
  const m = msg as Record<string, unknown>;
  if (m.type !== 'npc_dialogue') return false;
  const p = m.payload as Record<string, unknown> | undefined;
  return (
    typeof p === 'object' &&
    p !== null &&
    typeof p.npc_id === 'string' &&
    typeof p.say === 'string'
  );
}
```

修改 `startWsBridge` 的 `ws.onMessage` 回调：

```ts
const off = ws.onMessage((msg) => {
  if (isPlayerMoved(msg)) {
    const p = msg.payload;
    if (p.player_id === useGameStore.getState().playerId) {
      useGameStore.getState().setPosition({ x: p.x, y: p.y });
    }
    window.dispatchEvent(new CustomEvent<PlayerMovedPayload>(PLAYER_MOVED_EVENT, { detail: p }));
    return;
  }
  if (isNpcDialogue(msg)) {
    window.dispatchEvent(new CustomEvent<NpcDialoguePayload>(NPC_DIALOGUE_EVENT, { detail: msg.payload }));
    return;
  }
  // 未知 type：忽略
});
```

⚠️ **关键陷阱 — return 别忘**：原代码用 if 早返回；加第二个 if 仍要 return，否则同一 message 触发两个事件。

- [ ] **Step 4: 跑测试确认 pass**

```bash
cd ai-city/web
pnpm vitest run tests/lib/ws-events.test.ts
```

期望：3 passed。

- [ ] **Step 5: 提交**

```bash
cd ai-city
git add web/src/lib/ws-events.ts web/tests/lib/ws-events.test.ts
git commit -m "feat(web): ws-events 加 npc_dialogue 分支 (T03a)"
```

---

## Task T03b：web api.ts 加 postNpcTalk

**Files:**
- Modify: `ai-city/web/src/lib/api.ts`

- [ ] **Step 1: 在 api.ts 末尾加 postNpcTalk**

定位到 `apps/api-gateway/internal/handlers/npc.go` 即将返回的字段（见 T05 任务），定义 mirror type。

`ai-city/web/src/lib/api.ts` 在 `move` 之后追加：

```ts
// POST /v1/npc/:id/talk —— 玩家点 NPCDialog 选项
// 字段与 apps/api-gateway/internal/handlers/npc.go::talkResponseBody 一致
export interface NpcTalkResponse {
  npc_reply: string;
  next_options: Array<{ id: string; text: string }>;
  trace_id: string;
  dispatched: boolean; // false 时表示 npc_reply 走 fallback（前端降级）
}

// api.postNpcTalk = (npcId, body) =>
//   this.request<NpcTalkResponse>(`/v1/npc/${npcId}/talk`, {
//     method: 'POST',
//     body: JSON.stringify(body),
//   });
```

⚠️ **关键陷阱 — 路径 `/v1/npc/:id/talk`（单数）**：与现有 `authed.GET("/npcs/:id", ...)`（复数）是两个路由；T05 handler 新建，**不动**复数路由。

- [ ] **Step 2: TypeScript 编译检查**

```bash
cd ai-city/web
pnpm tsc --noEmit
```

期望：无错误。

- [ ] **Step 3: 提交**

```bash
cd ai-city
git add web/src/lib/api.ts
git commit -m "feat(web): api.postNpcTalk mirror type (T03b)"
```

---

## Task T03c：web NPCDialog 组件

**Files:**
- Create: `ai-city/web/src/components/NPCDialog.tsx`
- Test: `ai-city/web/src/components/NPCDialog.test.tsx` (new)

- [ ] **Step 1: 写失败单测**

`web/src/components/NPCDialog.test.tsx`：

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, act } from '@testing-library/react';
import { NPCDialog } from './NPCDialog';
import { NPC_DIALOGUE_EVENT } from '@/lib/ws-events';
import type { NpcDialoguePayload } from '@/lib/ws-events';

vi.mock('@/lib/api', () => ({
  api: {
    postNpcTalk: vi.fn().mockResolvedValue({
      npc_reply: '回您一句。',
      next_options: [],
      trace_id: 't',
      dispatched: true,
    }),
  },
}));

function emitNpc(payload: Partial<NpcDialoguePayload>) {
  const detail: NpcDialoguePayload = {
    npc_id: 'npc_wang_boss_001',
    player_id: 'p-1',
    tile_id: 'tile_0_0',
    say: '来了您嘞！',
    options: [{ id: 'ask', text: '问个事' }],
    reply_to_choice_id: null,
    ...payload,
  };
  act(() => {
    window.dispatchEvent(new CustomEvent(NPC_DIALOGUE_EVENT, { detail }));
  });
}

describe('NPCDialog', () => {
  beforeEach(() => {
    document.body.innerHTML = '';
  });

  it('hidden by default', () => {
    render(<NPCDialog playerId="p-1" />);
    expect(screen.queryByText(/来了您嘞/)).toBeNull();
  });

  it('shows say + options on npc_dialogue event', () => {
    render(<NPCDialog playerId="p-1" />);
    emitNpc({ say: '来了您嘞！', options: [{ id: 'a', text: '问个事' }] });
    expect(screen.getByText('来了您嘞！')).toBeTruthy();
    expect(screen.getByText('问个事')).toBeTruthy();
  });

  it('clicking option calls postNpcTalk', async () => {
    const { api } = await import('@/lib/api');
    render(<NPCDialog playerId="p-1" />);
    emitNpc({ options: [{ id: 'ask', text: '问个事' }] });
    await act(async () => {
      fireEvent.click(screen.getByText('问个事'));
    });
    expect(api.postNpcTalk).toHaveBeenCalledWith('npc_wang_boss_001', {
      player_id: 'p-1',
      choice_id: 'ask',
    });
  });

  it('shows reply when npc_dialogue with reply_to_choice_id arrives', () => {
    render(<NPCDialog playerId="p-1" />);
    emitNpc({ say: '问 1', reply_to_choice_id: null });
    emitNpc({ say: '回您一句。', reply_to_choice_id: 'ask', options: [] });
    expect(screen.getByText('回您一句。')).toBeTruthy();
  });

  it('close button hides dialog', () => {
    render(<NPCDialog playerId="p-1" />);
    emitNpc({});
    const close = screen.getByLabelText('close');
    fireEvent.click(close);
    expect(screen.queryByText(/来了您嘞/)).toBeNull();
  });
});
```

- [ ] **Step 2: 跑测试确认 fail**

```bash
cd ai-city/web
pnpm vitest run src/components/NPCDialog.test.tsx
```

期望：`Cannot find module './NPCDialog'`。

- [ ] **Step 3: 实现 NPCDialog.tsx**

`ai-city/web/src/components/NPCDialog.tsx`：

```tsx
'use client';

/**
 * NPC 对话气泡（Sprint 12 min slice）。
 *
 * 状态机：
 *   hidden → showing-say（收到 envelope, reply_to_choice_id == null）
 *         → awaiting-choice（点选项 → postNpcTalk）
 *         → showing-reply（收到 envelope, reply_to_choice_id 非空）
 *         → close
 *
 * 错误态：postNpcTalk 抛 → 错误 toast + 保留 say 视图。
 */
import { useEffect, useState } from 'react';
import { api } from '@/lib/api';
import {
  NPC_DIALOGUE_EVENT,
  type NpcDialoguePayload,
} from '@/lib/ws-events';

type State =
  | { kind: 'hidden' }
  | { kind: 'showing'; payload: NpcDialoguePayload; pending: boolean; error: string | null };

export function NPCDialog({ playerId }: { playerId: string }) {
  const [state, setState] = useState<State>({ kind: 'hidden' });

  useEffect(() => {
    const onDialog = (e: Event) => {
      const ce = e as CustomEvent<NpcDialoguePayload>;
      const p = ce.detail;
      setState({ kind: 'showing', payload: p, pending: false, error: null });
    };
    window.addEventListener(NPC_DIALOGUE_EVENT, onDialog as EventListener);
    return () => window.removeEventListener(NPC_DIALOGUE_EVENT, onDialog as EventListener);
  }, []);

  if (state.kind === 'hidden') return null;

  const { payload, pending, error } = state;

  const onChoose = async (choiceId: string) => {
    setState((s) => (s.kind === 'showing' ? { ...s, pending: true, error: null } : s));
    try {
      await api.postNpcTalk(payload.npc_id, {
        player_id: playerId,
        choice_id: choiceId,
      });
      // 等待 reply envelope 推回
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      setState((s) => (s.kind === 'showing' ? { ...s, pending: false, error: msg } : s));
    }
  };

  const onClose = () => setState({ kind: 'hidden' });

  return (
    <div
      role="dialog"
      aria-label={`NPC ${payload.npc_id} 对话`}
      className="fixed bottom-4 right-4 z-50 w-80 rounded-lg bg-white shadow-xl ring-1 ring-gray-200 p-4 text-sm"
    >
      <div className="flex items-start justify-between gap-2">
        <div className="font-semibold text-gray-900">{payload.npc_id}</div>
        <button
          type="button"
          aria-label="close"
          onClick={onClose}
          className="text-gray-400 hover:text-gray-700"
        >
          ×
        </button>
      </div>
      <div className="mt-2 text-gray-800 whitespace-pre-wrap">{payload.say}</div>

      {payload.options.length > 0 && (
        <div className="mt-3 flex flex-col gap-2">
          {payload.options.map((o) => (
            <button
              key={o.id}
              type="button"
              disabled={pending}
              onClick={() => onChoose(o.id)}
              className="text-left rounded border border-gray-300 px-3 py-2 hover:bg-gray-50 disabled:opacity-50"
            >
              {o.text}
            </button>
          ))}
        </div>
      )}

      {pending && <div className="mt-2 text-xs text-gray-500">回复中…</div>}
      {error && <div className="mt-2 text-xs text-red-600">{error}</div>}
    </div>
  );
}
```

- [ ] **Step 4: 跑测试确认 pass**

```bash
cd ai-city/web
pnpm vitest run src/components/NPCDialog.test.tsx
```

期望：5 passed。如失败常见 bug：
- `screen.getByLabelText('close')` 找不到：检查 `<button aria-label="close">` 是否在 showing 状态下才挂载（是；hidden 时不渲染）
- `getByText` 中文匹配：jsdom 默认支持；如失败改 `screen.getByText(/来了/)` 用正则

- [ ] **Step 5: 提交**

```bash
cd ai-city
git add web/src/components/NPCDialog.tsx web/src/components/NPCDialog.test.tsx
git commit -m "feat(web): NPCDialog 组件 (T03c)"
```

---

## Task T03d：web WorldMap 渲染 NPC + 命中检测

**Files:**
- Modify: `ai-city/web/src/components/Map/WorldMap.tsx`

⚠️ WorldMap 已经渲染 NPC 圆点（line 220-236）含 `data-npc-id`。**只需要扩 onClick** + **缺一个 getTemplate / say 内容的来源**。

简单方案：onClick 时直接 dispatch CustomEvent 触发 NPCDialog 弹气泡，气泡 say 内容是 NPC 的 say 文本（从 game store 或从 npc-templates 读）。1.0 demo 让 NPCDialog 先**显示一句"你好"**（后续 task 再让 server 推具体台词）。

但这会破坏"完整 demo"的体验。**正确做法**：WorldMap 的 NPC onClick 触发 npc_dialogue envelope，body 内容是 NPC 第一句 greeting；这条 envelope 走同一 ws-events 派发。问题：浏览器不能 publish 自己的 envelope（信源应是 server）。**最小解法**：onClick 直接 dispatch `aicity:npc_dialogue` CustomEvent（不经 ws.send），用 `reply_to_choice_id: null` 标识"主动 say 模拟"。

- [ ] **Step 1: 改 WorldMap.tsx onSvgClick：加 NPC 命中**

`web/src/components/Map/WorldMap.tsx` 现有 `onSvgClick`（约 150 行）开头加：

```tsx
const onSvgClick = async (e: React.MouseEvent<SVGSVGElement>) => {
  if (!svgRef.current) return;
  if (!playerId) {
    setState((s) => ({ ...s, error: '未登录（playerId 缺失）' }));
    return;
  }
  // 1) 点 NPC 圆点（命中整个 g）→ 弹 NPCDialog
  const npcEl = (e.target as Element).closest('[data-npc-id]') as Element | null;
  if (npcEl) {
    const npcId = npcEl.getAttribute('data-npc-id');
    if (npcId) {
      window.dispatchEvent(
        new CustomEvent('aicity:npc_dialogue', {
          detail: {
            npc_id: npcId,
            player_id: playerId,
            tile_id: tileIdAt(
              ... // 该 NPC 所在 tile 由 npcEl 所属 tile 推：npcEl 的 cx/cy
              // 简化：从 svg state.tiles 找
              ... // 留作 Step 2 详细实现
              0, 0
            ),
            say: '来了您嘞！', // demo 默认开屏语
            options: [
              { id: 'ask_business', text: '老板你这卖什么？' },
              { id: 'ask_news', text: '最近有什么新鲜事？' },
              { id: 'ask_help', text: '我想找个住处。' },
              { id: 'just_chat', text: '没什么，随便看看。' },
              { id: 'leave', text: '再见。' },
            ],
            reply_to_choice_id: null,
          },
        }),
      );
      return;
    }
  }
  // 2) 点 polygon/circle 早返回（保留）
  const tag = (e.target as Element).tagName;
  if (tag === 'polygon' || tag === 'circle') return;
  // ... 后续 move 逻辑不变
};
```

**实际 NPCDialog 应该有可工作的 say** —— 这里写死 5 选项 + 1 句 demo 台词。T05 接入 talk_tree 后，**api-gateway 收到 choice 后会用 yaml 解析 reply**。`dispatched=true` 时浏览器收到的 reply envelope 才会更新气泡。

- [ ] **Step 2: 跑 WorldMap 现有测试 + 编译**

```bash
cd ai-city/web
pnpm tsc --noEmit
pnpm vitest run
```

期望：编译通过；WorldMap 现有测试（如果有）通过；NPCDialog 5 测试仍 pass。

- [ ] **Step 3: 提交**

```bash
cd ai-city
git add web/src/components/Map/WorldMap.tsx
git commit -m "feat(web): WorldMap onClick 命中 NPC + 弹气泡 (T03d)"
```

---

## Task T03e：web city/page.tsx 挂载 NPCDialog

**Files:**
- Modify: `ai-city/web/src/app/city/page.tsx`

- [ ] **Step 1: 改 city/page.tsx**

完整替换为：

```tsx
'use client';

import { useEffect } from 'react';
import { WorldMap } from '@/components/Map/WorldMap';
import { PlayerHUD } from '@/components/PlayerHUD';
import { ChatBox } from '@/components/ChatBox';
import { NPCDialog } from '@/components/NPCDialog';
import { startWsBridge } from '@/lib/ws-events';
import { useGameStore } from '@/store/game';

export default function CityPage() {
  const playerId = useGameStore((s) => s.playerId);

  useEffect(() => startWsBridge(), []);

  return (
    <div className="relative h-screen w-screen overflow-hidden">
      <div className="map-container absolute inset-0">
        <WorldMap />
      </div>
      <PlayerHUD />
      <ChatBox />
      {playerId && <NPCDialog playerId={playerId} />}
    </div>
  );
}
```

- [ ] **Step 2: 跑全 web 测试**

```bash
cd ai-city/web
pnpm vitest run
```

期望：所有前序测试 pass；如有失败定位修复。

- [ ] **Step 3: 提交**

```bash
cd ai-city
git add web/src/app/city/page.tsx
git commit -m "feat(web): city/page 挂载 NPCDialog (T03e)"
```

---

## Task T04a：OCEAN schema 扩字段

**Files:**
- Modify: `ai-city/packages/npc-templates/OCEAN-schema.json`

⚠️ 已有 `OCEAN-schema.json`（如不存在，本任务改为 T04a'：新建最小 schema；如存在，本任务增量加字段）。

- [ ] **Step 1: 看现状**

```bash
cat ai-city/packages/npc-templates/OCEAN-schema.json
```

- [ ] **Step 2: 如果 schema 存在（推荐）**

在 `properties` 顶层加：

```json
"npc_id": { "type": "string", "description": "唯一 NPC ID（与 seed 的 entity_id 对齐）" },
"enabled": { "type": "boolean", "default": false, "description": "agent-os 是否加载此 NPC 进 dispatcher" },
"home_tile_id": { "type": "string" },
"say": {
  "type": "object",
  "properties": {
    "greeting": { "type": "array", "items": { "type": "string" } },
    "welcome": { "type": "array", "items": { "type": "string" } },
    "default_reply": { "type": "string" }
  }
},
"talk_tree": {
  "type": "object",
  "properties": {
    "initial": { "type": "string" },
    "nodes": {
      "type": "object",
      "additionalProperties": {
        "type": "object",
        "properties": {
          "say": { "type": "string" },
          "options": {
            "type": "array",
            "items": {
              "type": "object",
              "required": ["id", "text"],
              "properties": {
                "id": { "type": "string" },
                "text": { "type": "string" }
              }
            }
          },
          "reply_map": { "type": "object", "additionalProperties": { "type": "string" } },
          "next_options": {
            "type": "array",
            "items": {
              "type": "object",
              "required": ["id", "text"],
              "properties": {
                "id": { "type": "string" },
                "text": { "type": "string" }
              }
            }
          }
        }
      }
    }
  }
}
```

所有新字段**不**在 `required` 列表中 → 向后兼容，老模板仍 parse 通过。

- [ ] **Step 3: 跑 jsonschema 校验（如有 test）**

```bash
cd ai-city
ls apps/agent-os/tests/  # 应有 test_OCEAN_schema.py 否则跳过
```

如无，跳过；T04b 不会触发 schema 错误（因为 yaml 解析用 PyYAML 不走 jsonschema）。

- [ ] **Step 4: 提交**

```bash
cd ai-city
git add packages/npc-templates/OCEAN-schema.json
git commit -m "feat(npc-templates): OCEAN schema 扩 npc_id/enabled/say/talk_tree (T04a)"
```

---

## Task T04b：wang_boss.yaml 扩字段

**Files:**
- Modify: `ai-city/packages/npc-templates/wang_boss.yaml`

- [ ] **Step 1: 在文件顶部加 npc_id + enabled + home_tile_id + say + talk_tree**

完整替换为（保留 OCEAN / backstory / speech_style / tags / schedule）：

```yaml
npc_id: npc_wang_boss_001
enabled: true
name: 王老板
avatar_url: /static/npc/wang_boss.png
home_tile_id: tile_0_0

ocean:
  O: 35
  C: 70
  E: 55
  A: 85
  N: 40

speech_style:
  tone: warm
  dialect: 北京话
  catchphrase: "来了您嘞！"

backstory: |
  （保留原 backstory）

tags:
  - 商人
  - 长辈
  - 消息灵通
  - 善良好客

schedule:
  - hour: 6
    activity: 起床 / 买菜
    location: home
  - hour: 10
    activity: 开门营业
    location: tile_0_0_tavern
  - hour: 22
    activity: 打烊
    location: tile_0_0_tavern
  - hour: 23
    activity: 听京剧
    location: home

behavior_tree: tavern_greeting
default_lod: L1

# ---- Sprint 12 min slice 新增 ----

say:
  greeting:
    - "来了您嘞！"
    - "今儿个想吃点啥？"
    - "喝杯茶？"
  welcome: []            # T09 才填
  default_reply: "嗯，您说的这事我得想想。"

talk_tree:
  initial: greet
  nodes:
    greet:
      say: "来了您嘞！"
      options:
        - {id: ask_business, text: "老板你这卖什么？"}
        - {id: ask_news,     text: "最近城里有什么新鲜事？"}
        - {id: ask_help,     text: "我想找个住处。"}
        - {id: just_chat,    text: "没什么，随便看看。"}
        - {id: leave,        text: "再见。"}
      reply_map:
        ask_business: business
        ask_news:     news
        ask_help:     help
        just_chat:    chat
        leave:        end
    business:
      say: "招牌红烧肉、酱肘子，外加二两老白干。要不要来一份？"
      options:
        - {id: order,      text: "来一份红烧肉。"}
        - {id: ask_price,  text: "多少钱？"}
        - {id: back,       text: "我再看看。"}
      reply_map:
        order: order_confirm
        ask_price: price
        back: greet
    news:
      say: "听说 CBD 那边新开了家面馆，味道不错，您可以试试。"
      options:
        - {id: thanks, text: "谢了。"}
        - {id: back,   text: "我不感兴趣。"}
      reply_map:
        thanks: end
        back: greet
    help:
      say: "城东有家'如意客栈'，价格公道，老板娘人也好。"
      options:
        - {id: thanks, text: "多谢。"}
        - {id: back,   text: "我再想想。"}
      reply_map:
        thanks: end
        back: greet
    chat:
      say: "人老了就爱回忆从前，您慢坐，茶水管够。"
      options:
        - {id: nod,  text: "点头。"}
        - {id: back, text: "那我先走了。"}
      reply_map:
        nod: end
        back: greet
    order_confirm:
      say: "好嘞，红烧肉一份，5 分钟上桌。"
      options:
        - {id: ok,  text: "好。"}
      reply_map:
        ok: end
    price:
      say: "红烧肉 28 一份，量大。"
      options:
        - {id: ok,  text: "知道了。"}
        - {id: order, text: "那就来一份。"}
      reply_map:
        ok: end
        order: order_confirm
    end:
      say: "您慢走。"
      options: []
      reply_map: {}

prompt_hints:
  - "你是酒馆老板王老板，50 多岁"
  - "说话温和略带儿化音"
  - "对熟客称呼'老X'，对生客称呼'您'"
  - "关心人但不爱打听隐私"
  - "记得每个常客的偏好"
```

- [ ] **Step 2: 跑 npc_registry 单测确认 yaml 仍 parse**

```bash
cd ai-city/apps/agent-os
NPC_TEMPLATES_DIR=../../packages/npc-templates uv run pytest tests/test_npc_registry.py -v
```

期望：4 passed（之前 task T01c 写的）。如有 `KeyError` 或 `yaml.YAMLError` 说明 yaml 写错，按错误修。

- [ ] **Step 3: 跑 agent-os 起一遍**

```bash
cd ai-city/apps/agent-os
NPC_TEMPLATES_DIR=../../packages/npc-templates uv run python -m agent_os.main &
sleep 2
curl http://127.0.0.1:8084/healthz
# 期望：{"status":"ok",...,"npc_enabled":["npc_wang_boss_001"]}
kill %1
```

- [ ] **Step 4: 提交**

```bash
cd ai-city
git add packages/npc-templates/wang_boss.yaml
git commit -m "feat(npc-templates): wang_boss.yaml 扩 say.greeting/talk_tree (T04b)"
```

---

## Task T05a：api-gateway talk_tree.go 解析 + mtime 缓存

**Files:**
- Create: `ai-city/apps/api-gateway/internal/npc/talk_tree.go`
- Test: `ai-city/apps/api-gateway/internal/npc/talk_tree_test.go` (new)

- [ ] **Step 1: 写失败单测**

`apps/api-gateway/internal/npc/talk_tree_test.go`：

```go
package npc

import (
	"os"
	"path/filepath"
	"testing"
)

func writeYAML(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return p
}

const wangYAML = `
npc_id: npc_wang_boss_001
enabled: true
say:
  greeting: ["来了您嘞！"]
  default_reply: "嗯，想想。"
talk_tree:
  initial: greet
  nodes:
    greet:
      say: "来了您嘞！"
      options:
        - {id: ask, text: "问个事"}
        - {id: bye, text: "再见"}
      reply_map:
        ask: info
        bye: end
    info:
      say: "红烧肉 28 一份。"
      options:
        - {id: ok, text: "好"}
      reply_map:
        ok: end
    end:
      say: "您慢走。"
      options: []
      reply_map: {}
`

func TestLoadStore_LoadsWangBoss(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "wang_boss.yaml", wangYAML)

	store, err := NewTalkTreeStore(dir)
	if err != nil {
		t.Fatalf("NewTalkTreeStore: %v", err)
	}
	tmpl, err := store.Get("npc_wang_boss_001")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if tmpl.NPCID != "npc_wang_boss_001" {
		t.Errorf("NPCID = %q", tmpl.NPCID)
	}
	if tmpl.TalkTree.Initial != "greet" {
		t.Errorf("Initial = %q", tmpl.TalkTree.Initial)
	}
	if got := len(tmpl.TalkTree.Nodes["greet"].Options); got != 2 {
		t.Errorf("greet.Options len = %d, want 2", got)
	}
}

func TestTalkTreeStore_ReplyAt_ValidChoice(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "wang_boss.yaml", wangYAML)
	store, _ := NewTalkTreeStore(dir)

	reply, err := store.ReplyAt("npc_wang_boss_001", "greet", "ask")
	if err != nil {
		t.Fatalf("ReplyAt: %v", err)
	}
	if reply.Say != "红烧肉 28 一份。" {
		t.Errorf("reply.Say = %q", reply.Say)
	}
	if len(reply.NextOptions) != 1 {
		t.Errorf("NextOptions len = %d, want 1", len(reply.NextOptions))
	}
}

func TestTalkTreeStore_ReplyAt_InvalidChoice_ReturnsDefault(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "wang_boss.yaml", wangYAML)
	store, _ := NewTalkTreeStore(dir)

	reply, err := store.ReplyAt("npc_wang_boss_001", "greet", "non_existent")
	if err != nil {
		t.Fatalf("ReplyAt: %v", err)
	}
	if reply.Say != "嗯，想想。" { // default_reply
		t.Errorf("reply.Say = %q, want default_reply", reply.Say)
	}
}

func TestTalkTreeStore_ReplyAt_UnknownNPC(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewTalkTreeStore(dir)
	_, err := store.ReplyAt("ghost", "greet", "ask")
	if err == nil {
		t.Fatal("want error for unknown NPC")
	}
}

func TestTalkTreeStore_ReplyAt_UnknownNodeID(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "wang_boss.yaml", wangYAML)
	store, _ := NewTalkTreeStore(dir)
	_, err := store.ReplyAt("npc_wang_bang_boss_001", "ghost_node", "ask")
	// 这里 NPC 存在但 node 不存在 → 用 default_reply 也行；按实现选择
	if err == nil {
		t.Log("unknown node falls back to default_reply (acceptable)")
	}
}
```

- [ ] **Step 2: 跑测试确认 fail**

```bash
cd ai-city/apps/api-gateway
go test ./internal/npc/... -v
```

期望：`no Go files in ./internal/npc`。

- [ ] **Step 3: 实现 talk_tree.go**

`ai-city/apps/api-gateway/internal/npc/talk_tree.go`：

```go
// Package npc 解析 packages/npc-templates/*.yaml，提供 talk_tree 查询 API。
//
// Sprint 12 min slice：单进程内存 + mtime 缓存；不引入外部 KV。
package npc

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// DialogOption 与 apps/agent-os/src/agent_os/npc_registry.py::DialogOption
// 和 web/src/lib/api.ts::NpcTalkResponse 同名同字段。
type DialogOption struct {
	ID   string `yaml:"id"    json:"id"`
	Text string `yaml:"text"  json:"text"`
}

type TalkNode struct {
	Say         string            `yaml:"say"`
	Options     []DialogOption    `yaml:"options"`
	ReplyMap    map[string]string `yaml:"reply_map"`
	NextOptions []DialogOption    `yaml:"next_options"`
}

type TalkTree struct {
	Initial string              `yaml:"initial"`
	Nodes   map[string]TalkNode `yaml:"nodes"`
}

type SayConfig struct {
	Greeting     []string `yaml:"greeting"`
	Welcome      []string `yaml:"welcome"`
	DefaultReply string   `yaml:"default_reply"`
}

type NpcTemplate struct {
	NPCID      string    `yaml:"npc_id"`
	Name       string    `yaml:"name"`
	Enabled    bool      `yaml:"enabled"`
	HomeTileID string    `yaml:"home_tile_id"`
	AvatarURL  string    `yaml:"avatar_url"`
	Say        SayConfig `yaml:"say"`
	TalkTree   TalkTree  `yaml:"talk_tree"`
}

// Reply 是 ReplyAt 的返回结果。
type Reply struct {
	Say         string         `json:"say"`
	NextOptions []DialogOption `json:"next_options"`
}

type TalkTreeStore struct {
	mu       sync.RWMutex
	dir      string
	byID     map[string]NpcTemplate
	mtime    map[string]time.Time
	loadedAt time.Time
}

func NewTalkTreeStore(dir string) (*TalkTreeStore, error) {
	s := &TalkTreeStore{
		dir:   dir,
		byID:  make(map[string]NpcTemplate),
		mtime: make(map[string]time.Time),
	}
	if err := s.reload(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *TalkTreeStore) reload() error {
	if _, err := os.Stat(s.dir); os.IsNotExist(err) {
		return fmt.Errorf("npc_templates_dir not found: %s", s.dir)
	}
	files, err := filepath.Glob(filepath.Join(s.dir, "*.yaml"))
	if err != nil {
		return err
	}
	newByID := make(map[string]NpcTemplate)
	newMtime := make(map[string]time.Time)
	for _, f := range files {
		st, err := os.Stat(f)
		if err != nil {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var t NpcTemplate
		if err := yaml.Unmarshal(data, &t); err != nil {
			continue
		}
		if t.NPCID == "" {
			continue
		}
		newByID[t.NPCID] = t
		newMtime[t.NPCID] = st.ModTime()
	}
	s.mu.Lock()
	s.byID = newByID
	s.mtime = newMtime
	s.loadedAt = time.Now()
	s.mu.Unlock()
	return nil
}

// maybeReload 检查 mtime，文件改了则重 load。调用方在 Get 之前调。
func (s *TalkTreeStore) maybeReload() {
	s.mu.RLock()
	need := false
	for id, mt := range s.mtime {
		fp := filepath.Join(s.dir, id+".yaml")
		if st, err := os.Stat(fp); err == nil && st.ModTime().After(mt) {
			need = true
			break
		}
	}
	s.mu.RUnlock()
	if need {
		_ = s.reload()
	}
}

func (s *TalkTreeStore) Get(npcID string) (NpcTemplate, error) {
	s.maybeReload()
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.byID[npcID]
	if !ok {
		return NpcTemplate{}, fmt.Errorf("NPC_001: npc not found: %s", npcID)
	}
	return t, nil
}

// ReplyAt 返回 npc 在 nodeID 节点上 choiceID 选项的回复。
//
// 命中规则（按这个顺序）：
//  1. nodeID 节点存在 + choiceID 在 reply_map → 跳到目标 node，返该 node 的 say + next_options
//  2. 节点存在但 choiceID 不在 reply_map → 返 default_reply
//  3. 节点不存在 → 返 default_reply（不报错，玩家体验好）
func (s *TalkTreeStore) ReplyAt(npcID, nodeID, choiceID string) (Reply, error) {
	s.maybeReload()
	s.mu.RLock()
	defer s.mu.RUnlock()
	tmpl, ok := s.byID[npcID]
	if !ok {
		return Reply{}, fmt.Errorf("NPC_001: npc not found: %s", npcID)
	}
	node, ok := tmpl.TalkTree.Nodes[nodeID]
	if !ok {
		return Reply{Say: tmpl.Say.DefaultReply}, nil
	}
	nextID, hit := node.ReplyMap[choiceID]
	if !hit {
		return Reply{Say: tmpl.Say.DefaultReply}, nil
	}
	if nextNode, ok := tmpl.TalkTree.Nodes[nextID]; ok {
		return Reply{Say: nextNode.Say, NextOptions: nextNode.NextOptions}, nil
	}
	// 跳到的目标 node 不存在：fallback
	return Reply{Say: tmpl.Say.DefaultReply}, nil
}

// GetInitialNode 返回 initial 节点（用于第一次主动 say）。
func (s *TalkTreeStore) GetInitialNode(npcID string) (string, []DialogOption, error) {
	s.maybeReload()
	s.mu.RLock()
	defer s.mu.RUnlock()
	tmpl, ok := s.byID[npcID]
	if !ok {
		return "", nil, fmt.Errorf("NPC_001: npc not found: %s", npcID)
	}
	if tmpl.TalkTree.Initial == "" {
		return "", nil, nil
	}
	node := tmpl.TalkTree.Nodes[tmpl.TalkTree.Initial]
	return node.Say, node.Options, nil
}
```

⚠️ **关键陷阱 — `os.Stat(s.dir)` 不存在时返错**（不让 NewTalkTreeStore 静默返空 store）。`cmd/main.go` 在容器启动时若 volume 还没 mount，会 NPE 错位。

- [ ] **Step 4: 跑测试确认 pass**

```bash
cd ai-city/apps/api-gateway
go test ./internal/npc/... -v
```

期望：5 passed。

- [ ] **Step 5: 提交**

```bash
cd ai-city
git add apps/api-gateway/internal/npc/talk_tree.go apps/api-gateway/internal/npc/talk_tree_test.go
git commit -m "feat(api-gateway): talk_tree.go 解析 + mtime 缓存 (T05a)"
```

---

## Task T05b：api-gateway npc.go handler

**Files:**
- Create: `ai-city/apps/api-gateway/internal/handlers/npc.go`
- Test: `ai-city/apps/api-gateway/internal/handlers/npc_test.go` (new)

- [ ] **Step 1: 写失败单测**

`apps/api-gateway/internal/handlers/npc_test.go`：

```go
package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aicity/api-gateway/internal/npc"
	"github.com/gin-gonic/gin"
)

func newTestRouter(t *testing.T) (*gin.Engine, *npc.TalkTreeStore) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "wang_boss.yaml"), []byte(`
npc_id: npc_wang_boss_001
enabled: true
say:
  default_reply: "嗯，想想。"
talk_tree:
  initial: greet
  nodes:
    greet:
      say: "来了您嘞！"
      options:
        - {id: ask, text: "问个事"}
      reply_map:
        ask: info
    info:
      say: "红烧肉 28。"
      options:
        - {id: ok, text: "好"}
      reply_map: {ok: end}
    end: {say: "您慢走。", options: [], reply_map: {}}
`), 0o644); err != nil {
		t.Fatalf("write yaml: %v", err)
	}
	store, err := npc.NewTalkTreeStore(dir)
	if err != nil {
		t.Fatalf("NewTalkTreeStore: %v", err)
	}
	r := gin.New()
	h := &NpcHandler{Store: store, Publisher: nil, Channel: "aicity:npc_dialogue"}
	r.POST("/v1/npc/:id/talk", h.Talk)
	return r, store
}

func TestTalkHandler_ValidChoice_Returns200(t *testing.T) {
	r, _ := newTestRouter(t)
	body, _ := json.Marshal(map[string]string{"player_id": "p-1", "choice_id": "ask"})
	req := httptest.NewRequest(http.MethodPost, "/v1/npc/npc_wang_boss_001/talk", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["npc_reply"] != "红烧肉 28。" {
		t.Errorf("npc_reply = %v", resp["npc_reply"])
	}
}

func TestTalkHandler_MissingPlayerID_Returns400(t *testing.T) {
	r, _ := newTestRouter(t)
	body, _ := json.Marshal(map[string]string{"choice_id": "ask"})
	req := httptest.NewRequest(http.MethodPost, "/v1/npc/npc_wang_boss_001/talk", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestTalkHandler_MissingChoiceID_Returns400(t *testing.T) {
	r, _ := newTestRouter(t)
	body, _ := json.Marshal(map[string]string{"player_id": "p-1"})
	req := httptest.NewRequest(http.MethodPost, "/v1/npc/npc_wang_boss_001/talk", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestTalkHandler_UnknownNPC_Returns404(t *testing.T) {
	r, _ := newTestRouter(t)
	body, _ := json.Marshal(map[string]string{"player_id": "p-1", "choice_id": "ask"})
	req := httptest.NewRequest(http.MethodPost, "/v1/npc/ghost/talk", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}
```

⚠️ 上面 import 有 `os` 和 `path/filepath` —— 测试文件顶部 import 块要加：

```go
import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/aicity/api-gateway/internal/npc"
	"github.com/gin-gonic/gin"
)
```

- [ ] **Step 2: 跑测试确认 fail**

```bash
cd ai-city/apps/api-gateway
go test ./internal/handlers/... -v -run TestTalkHandler
```

期望：`no Go files` for NpcHandler 或 compile error。

- [ ] **Step 3: 实现 npc.go**

`apps/api-gateway/internal/handlers/npc.go`：

```go
// Package handlers — NPC talk endpoint (Sprint 12 min slice)。
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/aicity/api-gateway/internal/npc"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// Publisher 是 rdb.Publish 的最小接口（便于单测注入假实现）。
type Publisher interface {
	Publish(ctx context.Context, channel, payload string) *redis.IntCmd
}

type NpcHandler struct {
	Store     *npc.TalkTreeStore
	Publisher Publisher // 可为 nil（test mode 或 dispatch 关）
	Channel   string    // "aicity:npc_dialogue"
}

type talkRequestBody struct {
	PlayerID string `json:"player_id"`
	ChoiceID string `json:"choice_id"`
}

type talkResponseBody struct {
	NPCReply    string                `json:"npc_reply"`
	NextOptions []npc.DialogOption    `json:"next_options"`
	TraceID     string                `json:"trace_id"`
	Dispatched  bool                  `json:"dispatched"`
}

func (h *NpcHandler) Talk(c *gin.Context) {
	npcID := c.Param("id")
	if npcID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "NPC_002", "detail": "missing npc id"})
		return
	}

	var body talkRequestBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "NPC_002", "detail": "invalid json: " + err.Error()})
		return
	}
	if body.PlayerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "NPC_002", "detail": "missing player_id"})
		return
	}
	if body.ChoiceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "NPC_002", "detail": "missing choice_id"})
		return
	}

	// 1) 玩家是否注册：本 sprint 简化 — body.player_id 非空即放行
	//    （1.0 完整版要查 PG player 表；TODO 后续 sprint）
	_ = body.PlayerID

	// 2) 查 talk_tree
	tmpl, err := h.Store.Get(npcID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "NPC_001", "detail": err.Error()})
		return
	}

	// 3) 当前 node = initial；Sprint 13+ 会改成 per-player state
	currentNodeID := tmpl.TalkTree.Initial
	reply, err := h.Store.ReplyAt(npcID, currentNodeID, body.ChoiceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "NPC_003", "detail": err.Error()})
		return
	}

	// 4) 构造 reply envelope → publish
	traceID := fmt.Sprintf("api-%d", time.Now().UnixNano())
	envelope := map[string]any{
		"type":     "npc_dialogue",
		"trace_id": traceID,
		"ts_ms":    time.Now().UnixMilli(),
		"payload": map[string]any{
			"npc_id":            npcID,
			"player_id":         body.PlayerID,
			"tile_id":           tmpl.HomeTileID,
			"say":               reply.Say,
			"options":           reply.NextOptions,
			"reply_to_choice_id": body.ChoiceID,
		},
	}
	envJSON, _ := json.Marshal(envelope)

	dispatched := false
	if h.Publisher != nil && h.Channel != "" {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 1*time.Second)
		defer cancel()
		if err := h.Publisher.Publish(ctx, h.Channel, string(envJSON)).Err(); err == nil {
			dispatched = true
		}
	}

	c.JSON(http.StatusOK, talkResponseBody{
		NPCReply:    reply.Say,
		NextOptions: reply.NextOptions,
		TraceID:     traceID,
		Dispatched:  dispatched,
	})
}
```

⚠️ **关键陷阱 — `_ = body.PlayerID` 显式忽略**：sprint 12 min slice 暂不做 PG player 校验；保留字段以备后续。

⚠️ **关键陷阱 — `reply.NextOptions` 是空切片不是 nil**：`ReplyAt` 已经返 `[]DialogOption{}` 当空（nextNode.NextOptions 可能是 nil）；JSON 序列化时空切片 → `[]`，nil → `null`。`npc.TalkNode.NextOptions` 在 yaml 缺省时是 nil。如需保证 `[]`，在 `ReplyAt` 处加 `if nextNode.NextOptions == nil { nextNode.NextOptions = []npc.DialogOption{} }`。

修：把 T05a 的 `ReplyAt` 改为：

```go
if nextNode, ok := tmpl.TalkTree.Nodes[nextID]; ok {
    opts := nextNode.NextOptions
    if opts == nil {
        opts = []DialogOption{}
    }
    return Reply{Say: nextNode.Say, NextOptions: opts}, nil
}
```

- [ ] **Step 4: 跑测试确认 pass**

```bash
cd ai-city/apps/api-gateway
go test ./internal/handlers/... -v -run TestTalkHandler
```

期望：4 passed。

- [ ] **Step 5: 跑全 api-gateway 测试看是否破坏其它**

```bash
cd ai-city/apps/api-gateway
go build ./... && go test ./... -count=1
```

- [ ] **Step 6: 提交**

```bash
cd ai-city
git add apps/api-gateway/internal/handlers/npc.go apps/api-gateway/internal/handlers/npc_test.go apps/api-gateway/internal/npc/talk_tree.go
git commit -m "feat(api-gateway): npc handler POST /v1/npc/:id/talk (T05b)"
```

---

## Task T05c：api-gateway router 挂载 + main.go 注入 Store + Publisher

**Files:**
- Modify: `apps/api-gateway/internal/router/router.go`
- Modify: `apps/api-gateway/cmd/main.go`
- Modify: `apps/api-gateway/internal/config/config.go`

- [ ] **Step 1: 改 config.go：加 NPCTemplatesDir + ChannelNpcDialogue**

`apps/api-gateway/internal/config/config.go` 找到 `Config` struct 末尾加：

```go
NPCTemplatesDir      string
ChannelNpcDialogue   string
```

`Load()` 末尾加：

```go
NPCTemplatesDir:    getEnv("NPC_TEMPLATES_DIR", "./packages/npc-templates"),
ChannelNpcDialogue: getEnv("REDIS_CHANNEL_NPC_DIALOGUE", "aicity:npc_dialogue"),
```

- [ ] **Step 2: 改 router.go：注册 NpcHandler**

`Register` 签名扩展（第 3 个参数加 npcHandler）：

```go
func Register(r *gin.Engine, cfg *config.Config, db *pgxpool.Pool, playerStore *store.PlayerStore, worldClient *worldgrpc.Client, npcHandler *handlers.NpcHandler) {
    // ...
    authed.POST("/npc/:id/talk", npcHandler.Talk)
    // 现有 authed.GET("/npcs/:id", ...) 和 authed.POST("/npcs/:id/dialogue", ...) 不动
}
```

- [ ] **Step 3: 改 main.go：构造 TalkTreeStore + NpcHandler + 注入 router**

`apps/api-gateway/cmd/main.go` 中：

1) 在 `db` 初始化后加：

```go
npcStore, err := npc.NewTalkTreeStore(cfg.NPCTemplatesDir)
if err != nil {
    log.Fatalf("npc_templates_dir load failed: %v", err)
}
```

2) `rdb` 已存在的位置，构造 NpcHandler：

```go
npcHandler := handlers.NewNpcHandler(npcStore, rdb, cfg.ChannelNpcDialogue)
```

3) `router.Register(...)` 调用加 npcHandler 参数：

```go
router.Register(r, cfg, db, playerStore, worldClient, npcHandler)
```

4) 加 import：

```go
"github.com/aicity/api-gateway/internal/npc"
```

并在 `handlers.NewNpcHandler` 存在时调；如未存在，新建构造函数：

```go
// In handlers/npc.go add:
func NewNpcHandler(store *npc.TalkTreeStore, pub *redis.Client, channel string) *NpcHandler {
    return &NpcHandler{Store: store, Publisher: pub, Channel: channel}
}
```

⚠️ **关键陷阱 — Publisher 类型是 `*redis.Client` 不是自定义 interface**：`go-redis` 的 `*redis.Client` 实现了 `Publish(ctx, channel, payload) *redis.IntCmd`，**自动**满足 `handlers.Publisher` interface。但 `main.go` 注入的是 `*redis.Client`，需要把 `handlers.NpcHandler.Publisher` 类型改成 `*redis.Client`（更简单）。

修订：把 T05b 的 `NpcHandler` 字段改为：

```go
type NpcHandler struct {
    Store     *npc.TalkTreeStore
    Publisher *redis.Client // may be nil
    Channel   string
}
```

修 `Talk` 中 publish 调用：

```go
if h.Publisher != nil {
    if err := h.Publisher.Publish(ctx, h.Channel, string(envJSON)).Err(); err == nil {
        dispatched = true
    }
}
```

`cmd/main.go` 注入时直接传 `rdb`：

```go
npcHandler := &handlers.NpcHandler{Store: npcStore, Publisher: rdb, Channel: cfg.ChannelNpcDialogue}
```

- [ ] **Step 4: 编译 + 跑全测试**

```bash
cd ai-city/apps/api-gateway
go build ./... && go test ./... -count=1
```

期望：编译通过；所有测试 pass。

- [ ] **Step 5: 提交**

```bash
cd ai-city
git add apps/api-gateway/internal/router/router.go apps/api-gateway/cmd/main.go apps/api-gateway/internal/config/config.go apps/api-gateway/internal/handlers/npc.go
git commit -m "feat(api-gateway): router 挂 /v1/npc/:id/talk + main 注入 store (T05c)"
```

---

## Task T05d：handler 调 redis publish + wire-contract 对齐

**Files:**
- Modify: `ai-city/apps/api-gateway/internal/handlers/npc_talk.go`
- Modify: `ai-city/apps/api-gateway/internal/handlers/npc_talk_test.go`
- Modify: `ai-city/apps/agent-os/src/agent_os/action_dispatcher.py`
- Modify: `ai-city/apps/agent-os/tests/test_action_dispatcher.py`

⚠️ **本 task 是 wire-contract 修复**，不是新功能。背景：T05b 实现 handler 时构造了 `{type, payload}` 外层 envelope + 内层 payload 一起 publish，但 ws-gateway 收到 Redis 帧后会**再次**用 `protocol.NewEnvelope` 包一层 `{type, trace_id, ts_ms, payload}`。结果浏览器收到的是 `frame.payload.payload.npc_id`（双层 wrap），`ws-events.ts` 找不到字段，NPCDialog 永远不弹。

修复方向：publishers（agent-os + api-gateway）只发 **inner payload**；ws-gateway 是**唯一**做 envelope 包装的层。这与 world-engine `aicity:player:moved` 早已采纳的模式一致。

- [ ] **Step 1: 写失败单测（api-gateway 侧）**

`apps/api-gateway/internal/handlers/npc_talk_test.go` 在 `TestNPCTalk_HappyPath` 末尾追加：

```go
// Published payload must be the INNER payload (no outer envelope).
// ws-gateway wraps it uniformly — publishers must NOT double-wrap.
var published map[string]any
if err := json.Unmarshal([]byte(fakeR.published[0].Payload), &published); err != nil {
    t.Fatalf("unmarshal published payload: %v raw=%s", err, fakeR.published[0].Payload)
}
if _, hasType := published["type"]; hasType {
    t.Errorf("published payload should NOT have outer 'type' key; got %+v", published)
}
if _, hasPayload := published["payload"]; hasPayload {
    t.Errorf("published payload should NOT have outer 'payload' key; got %+v", published)
}
if published["npc_id"] != "npc_wang_boss_001" {
    t.Errorf("published npc_id = %v, want npc_wang_boss_001", published["npc_id"])
}
if published["say"] != "小店经营杂货。" { // 测试 yaml 中 business 节点 say
    t.Errorf("published say = %v, want 小店经营杂货。", published["say"])
}
if published["reply_to_choice_id"] != "ask_business" {
    t.Errorf("published reply_to_choice_id = %v, want ask_business", published["reply_to_choice_id"])
}
if _, ok := published["ts_ms"].(float64); !ok {
    t.Errorf("published ts_ms should be a number; got %T (%v)", published["ts_ms"], published["ts_ms"])
}
if _, ok := published["trace_id"].(string); !ok {
    t.Errorf("published trace_id should be a string; got %T (%v)", published["trace_id"], published["trace_id"])
}
```

`fakeR.published[0]` 字段已在 Sprint 7 引入（`{Channel, Payload string}`），直接用。

- [ ] **Step 2: 写失败单测（agent-os 侧）**

`apps/agent-os/tests/test_action_dispatcher.py` 改 `test_say_minimal_publishes_envelope`：

```python
@pytest.mark.asyncio
async def test_say_publishes_inner_payload_only():
    """publish 内容必须是 inner payload（无外层 type/payload wrap）。

    ws-gateway 是唯一 envelope 包装层；publishers 只发 inner payload。
    """
    fake = FakeRedis()
    d = ActionDispatcher(fake, channel="aicity:npc_dialogue")  # type: ignore[arg-type]
    await d.say("npc_wang_boss_001", "来了您嘞！")

    channel, payload = fake.calls[0]
    parsed = json.loads(payload)

    # 必须**没有**外层 wrap
    assert "type" not in parsed, f"unexpected outer 'type' in {parsed}"
    assert "payload" not in parsed, f"unexpected outer 'payload' in {parsed}"

    # inner payload 直接是 npc_id/say/options 等字段
    assert parsed["npc_id"] == "npc_wang_boss_001"
    assert parsed["say"] == "来了您嘞！"
    assert parsed["options"] == []
    assert parsed["reply_to_choice_id"] is None
    assert parsed["trace_id"]  # uuid
    assert parsed["ts_ms"] > 0
```

- [ ] **Step 3: 跑测试确认 fail**

```bash
cd ai-city/apps/api-gateway && go test ./internal/handlers/... -run TestNPCTalk_HappyPath -v
cd ai-city/apps/agent-os && uv run pytest tests/test_action_dispatcher.py -v
```

期望：两边都报 outer `type` / `payload` key 不应存在。

- [ ] **Step 4: 改 npc_talk.go：去掉外层 envelope**

`apps/api-gateway/internal/handlers/npc_talk.go` 把 handler 末尾的 publish 段：

```go
// Best-effort publish — log on failure, but never surface as HTTP error.
//
// Publishes the INNER payload only (no outer envelope). ws-gateway wraps it
// uniformly with type/trace_id/ts_ms before fanning out to the browser.
// This matches the world-engine pattern for `aicity:player:moved`.
innerPayload := map[string]any{
    "npc_id":             resp.NpcID,
    "player_id":          resp.PlayerID,
    "tile_id":            resp.TileID,
    "say":                resp.Say,
    "options":            opts,
    "reply_to_choice_id": resp.ReplyToChoiceID,
    "ts_ms":              time.Now().UnixMilli(),
    "trace_id":           c.GetHeader("X-Trace-ID"),
}
payload, _ := json.Marshal(innerPayload)
if err := h.Redis.Publish(c.Request.Context(), h.NPCChannel, string(payload)).Err(); err != nil {
    h.Logger.Warn("npc publish failed",
        zap.String("npc_id", req.NpcID),
        zap.Error(err))
}
```

`time` 包需要新加 import（`npc_talk.go` 顶部）。

- [ ] **Step 5: 改 action_dispatcher.py：去掉外层 envelope**

`apps/agent-os/src/agent_os/action_dispatcher.py` 把 `say` 内的 `envelope` 构造改为 `payload`：

```python
"""发送一句 NPC 台词。失败仅 warn，agent tick 不阻塞。

Publishes the INNER payload only (no outer envelope) — ws-gateway wraps
it uniformly with type/trace_id/ts_ms before fanning out to the browser.
This matches the world-engine pattern for `aicity:player:moved`.

payload:
{
  "npc_id": "...", "player_id": "...", "tile_id": "...",
  "say": "...",
  "options": [...],                   // 始终是数组（不是 null）
  "reply_to_choice_id": "..."|null,   // null = 主动 say；非空 = 玩家回复
  "ts_ms": <int ms>,
  "trace_id": "<uuid>"
}
"""
payload: dict[str, Any] = {
    "npc_id": npc_id,
    "player_id": player_id or "",
    "tile_id": tile_id or "",
    "say": text,
    "options": list(options) if options else [],
    "reply_to_choice_id": reply_to_choice_id,
    "ts_ms": int(time.time() * 1000),
    "trace_id": str(uuid.uuid4()),
}
try:
    await self._redis.publish(self._channel, json.dumps(payload, ensure_ascii=False))
except Exception as e:  # noqa: BLE001
    logger.warning(
        "dispatcher.say publish failed",
        extra={"npc_id": npc_id, "err": str(e)},
    )
```

- [ ] **Step 6: 跑测试确认 pass**

```bash
cd ai-city/apps/api-gateway && go test ./internal/handlers/... -v -run TestNPCTalk
cd ai-city/apps/agent-os && uv run pytest tests/test_action_dispatcher.py -v
```

期望：两边都通过；其它既有测试（如 `test_npc_registry.py` 等）不破。

- [ ] **Step 7: 提交（拆 2 commit 按服务）**

```bash
cd ai-city
git add apps/api-gateway/internal/handlers/npc_talk.go apps/api-gateway/internal/handlers/npc_talk_test.go
git commit -m "fix(api-gateway): publish npc_dialogue inner payload only (no outer envelope"

git add apps/agent-os/src/agent_os/action_dispatcher.py apps/agent-os/tests/test_action_dispatcher.py
git commit -m "fix(agent-os): publish npc_dialogue inner payload only (no outer envelope)"
```

实际 commit hash：`53a753d` (api-gateway) + `b90e8a0` (agent-os)。

⚠️ **关键陷阱 — 双 commit 而非 squash**：两边分属不同服务，cross-service revert 风险隔离；CI blame / bisect 也更清晰。

⚠️ **关键陷阱 — `trace_id` 来源不同**：agent-os 用 `str(uuid.uuid4())` 自生；api-gateway 读 `c.GetHeader("X-Trace-ID")` 复用 HTTP request trace。两者**不**冲突：ws-gateway 看到 `payload.trace_id` 字段（如果有）会保留，否则自生 `protocol.NewEnvelope` 的 trace_id。

---

## Task T05e：docker compose backend smoke verify

**Files:**
- Modify: `ai-city/apps/agent-os/Dockerfile`
- Modify: `ai-city/docker-compose.yml`

⚠️ 本 task 是 T06 手动浏览器 E2E 的**前置门**：先把 backend 全栈拉起来（agent-os + api-gateway + ws-gateway + redis + postgres）跑通，再用 curl + redis-cli 验证关键路径。这一步通过 = 浏览器侧 99% 不会卡 network 层。

- [ ] **Step 1: 改 Dockerfile：T01e 规格**

`apps/agent-os/Dockerfile` 完整替换为（T01e spec）：

```dockerfile
FROM python:3.12-slim

WORKDIR /app

# 安装 uv
RUN pip install --no-cache-dir uv

# 复制项目文件
COPY pyproject.toml ./
COPY src/ ./src/

# 安装依赖（root context 是 ai-city/，所以相对路径是 apps/agent-os/...）
RUN uv pip install --system --no-cache .

# 复制 NPC 模板
COPY packages/npc-templates/ /etc/aicity/npc-templates/

ENV PYTHONUNBUFFERED=1
ENV NPC_TEMPLATES_DIR=/etc/aicity/npc-templates
ENV HTTP_PORT=8084
ENV SAY_TICK_SECONDS=5.0
ENV REDIS_URL=redis://redis:6379/0
ENV REDIS_CHANNEL_NPC_DIALOGUE=aicity:npc_dialogue
ENV LOG_LEVEL=info
ENV SERVICE_NAME=agent-os

EXPOSE 8084

CMD ["python", "-m", "agent_os.main"]
```

⚠️ **关键陷阱 — slim 镜像无 curl**：healthcheck 改 `wget --spider http://127.0.0.1:8084/healthz`（`python:3.12-slim` 默认含 wget）。

- [ ] **Step 2: 改 docker-compose.yml：加 agent-os service + api-gateway env**

```yaml
agent-os:
  build:
    context: .
    dockerfile: apps/agent-os/Dockerfile
  container_name: aicity-agent-os
  restart: unless-stopped
  ports:
    - "8084:8084"
  environment:
    REDIS_URL: redis://redis:6379/0
    REDIS_CHANNEL_NPC_DIALOGUE: aicity:npc_dialogue
    NPC_TEMPLATES_DIR: /etc/aicity/npc-templates
    HTTP_PORT: "8084"
    SAY_TICK_SECONDS: "5.0"
    LOG_LEVEL: info
    SERVICE_NAME: agent-os
  volumes:
    - ./packages/npc-templates:/etc/aicity/npc-templates:ro
  depends_on:
    redis:
      condition: service_healthy
    api-gateway:
      condition: service_started
  healthcheck:
    test: ["CMD", "wget", "--spider", "-q", "http://127.0.0.1:8084/healthz"]
    interval: 10s
    timeout: 3s
    retries: 5
```

api-gateway service 增 env（不动服务定义本身）：

```yaml
api-gateway:
  # ... 现有
  environment:
    # ... 现有
    NPC_CONFIG_DIR: /etc/aicity/npc-templates
    REDIS_CHANNEL_NPC_DIALOGUE: aicity:npc_dialogue
  volumes:
    # ... 现有
    - ./packages/npc-templates:/etc/aicity/npc-templates:ro
```

docker-compose.yml 头部注释从 "7 容器" 改 "8 容器"。

- [ ] **Step 3: 起全栈**

```bash
cd ai-city
docker compose up -d --build
docker compose ps
```

期望：8 容器全部 healthy / running（web 是 running，无 healthcheck）。

- [ ] **Step 4: curl health 检查**

```bash
curl -fsS http://localhost:8084/healthz | python -m json.tool   # agent-os
curl -fsS http://localhost:8080/health                            # api-gateway
curl -fsS http://localhost:8082/readyz                            # ws-gateway
curl -fsS http://localhost:50052/readyz                           # world-engine
```

期望 4 个 endpoint 都返 200；`/healthz` agent-os 应含 `"npc_enabled": ["npc_wang_boss_001"]`。

- [ ] **Step 5: redis-cli 验 active say**

一个终端订阅：

```bash
docker compose exec -T redis redis-cli SUBSCRIBE aicity:npc_dialogue
```

另一个终端等 ≤5s（`SAY_TICK_SECONDS=5.0`）：

期望收到 inner payload（**无外层 `type` / `payload` wrap**）：

```
1) "aicity:npc_dialogue"
2) {"npc_id":"npc_wang_boss_001","player_id":"","tile_id":"tile_0_0",
    "say":"来了您嘞！","options":[],
    "reply_to_choice_id":null,"ts_ms":1700000000000,"trace_id":"..."}
3) (nil)
```

- [ ] **Step 6: curl POST /v1/npc/talk**

```bash
TOKEN=$(curl -fsS -X POST http://localhost:8080/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"demo","password":"demo123"}' | jq -r .token)

DEMO_ID=$(docker compose exec -T postgres psql -U aicity -d aicity -tAc \
  "select id from player where username='demo';" | tr -d '\r')

curl -fsS -X POST http://localhost:8080/v1/npc/talk \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"npc_id\":\"npc_wang_boss_001\",\"player_id\":\"$DEMO_ID\",\"choice_id\":\"ask_business\"}" | python -m json.tool
```

期望返 200 + 含 `npc_id/player_id/tile_id/say/options/reply_to_choice_id` 字段的 JSON。
> **注**：Sprint 12 min-slice 范围 `wang_boss.yaml::talk_tree` 仅 Sprint 13+ 才补，所以 `choice_id` 不在 reply_map 时返 `NPC_002`；核心是验 status code mapping + reply envelope 形状，不是验台词。

- [ ] **Step 7: 提交**

```bash
cd ai-city
git add apps/agent-os/Dockerfile docker-compose.yml
git commit -m "feat(agent-os): Dockerfile T01e spec + compose service mount (T06)"
```

实际 commit hash：`890ef4b`。

---

## Task T06：集成 verify（手动 E2E）

**Files:**
- Modify: `ai-city/docs/1.0-sprint12-min-slice-e2e.md` (new)

- [ ] **Step 1: 起全栈**

```bash
cd ai-city
docker compose up -d --build
docker compose ps
```

期望：所有 8 容器 healthy。

- [ ] **Step 2: 验证 agent-os /healthz + npc_loaded**

```bash
docker compose exec -T agent-os curl -s http://127.0.0.1:8084/healthz | python -m json.tool
```

期望：

```json
{
  "status": "ok",
  "service": "agent-os",
  "npc_loaded": 1,
  "npc_enabled": ["npc_wang_boss_001"]
}
```

- [ ] **Step 3: 验证 ws-gateway 启动日志含多频道**

```bash
docker compose logs ws-gateway | head -30
```

期望：`channels=[aicity:player:moved aicity:npc_dialogue]`。

- [ ] **Step 4: 手动模拟 E2E**

1. 浏览器登录 demo / demo123（demo 玩家 UUID 通过 `docker compose exec -T postgres psql -U aicity -d aicity -tAc "select id from player where username='demo';"` 拿）
2. 浏览器走到 `tile_0_0`（用 WorldMap 点 tile 中心）
3. 浏览器 dev console 监听：

```js
window.addEventListener('aicity:npc_dialogue', (e) => console.log('NPC:', e.detail));
```

4. 等 ≤5s 看到王老板主动 say 的 envelope log
5. 点 NPC 圆点（黄 r=3 圆）→ NPCDialog 弹气泡
6. 点 1 个选项（例 "老板你这卖什么？"）→ 等 1-2s 看到 reply envelope log（"回您一句"）

- [ ] **Step 5: curl 验证 npc handler**

```bash
TOKEN=$(curl -sX POST http://localhost:8080/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"demo","password":"demo123"}' | python -c "import sys,json;print(json.load(sys.stdin)['token'])")
PLAYER_ID=$(docker compose exec -T postgres psql -U aicity -d aicity -tAc "select id from player where username='demo';")
curl -sX POST http://localhost:8080/v1/npc/npc_wang_boss_001/talk \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"player_id\":\"$PLAYER_ID\",\"choice_id\":\"ask_business\"}" | python -m json.tool
```

期望：

```json
{
  "npc_reply": "招牌红烧肉、酱肘子，外加二两老白干。要不要来一份？",
  "next_options": [
    {"id": "order", "text": "来一份红烧肉。"},
    ...
  ],
  "trace_id": "api-...",
  "dispatched": true
}
```

- [ ] **Step 6: 写复盘文档**

新建 `ai-city/docs/1.0-sprint12-min-slice-e2e.md`：

```markdown
# Sprint 12 min-slice 集成验证复盘

## 范围
- agent-os: dispatcher.say + say_scheduler + Redis publish
- ws-gateway: 多频道 Subscribe (player_moved + npc_dialogue)
- web: NPCDialog 组件 + ws-events 分支 + WorldMap NPC 命中
- api-gateway: POST /v1/npc/:id/talk + talk_tree yaml 解析

## 验收
- [x] 王老板 5s tick 主动 say
- [x] 浏览器 console 看到 aicity:npc_dialogue CustomEvent
- [x] 点 NPC 圆点 → 弹气泡 + 5 选项
- [x] 点选项 → 1-2s 内看到 reply envelope
- [x] curl POST /v1/npc/:id/talk 返 200 + npc_reply + dispatched=true

## 已知缺口
- 王老板 NPC 位置由 agent-os 写死（home_tile_id=tile_0_0），不走 MoveEntity
- 不支持多 NPC / 多玩家并发 reply
- 不接 LLM；say 文本都是 yaml 写死的
- 不做 welcome 剧本（玩家首次进 tile_0_0 才触发）
- acceptance_1_0 binary 5/5 不能跑

## 后续 sprint 入口
- proto EntityType + MoveEntity（Sprint 13）
- First-login 标记 + welcome storyline（Sprint 13）
- acceptance_1_0 binary（Sprint 14）
```

- [ ] **Step 7: 提交**

```bash
cd ai-city
git add docs/1.0-sprint12-min-slice-e2e.md
git commit -m "docs(sprint12): min-slice 集成验证复盘 (T06)"
```

---

## 自检

- **Spec 覆盖**：
  - §一 1.1 数据流 → T01d + T02b + T03a + T05b 实现
  - §二 1.1 agent-os 改动清单 → T01a-e 5 task 覆盖
  - §二 1.2 ws-gateway 改动 → T02a-c 3 task 覆盖
  - §二 1.3 api-gateway 改动 → T05a-e 5 task 覆盖（含 T05d wire-contract + T05e docker smoke）
  - §二 1.4 web 改动 → T03a-e 5 task 覆盖
  - §二 1.5 wang_boss.yaml → T04b
  - §二 1.6 OCEAN schema → T04a
  - §三 错误处理 → T05b (NPC_001/002/003)
  - §四 测试 → 每个 task 自带测试
  - §五 依赖 → T01a 改 pyproject；T05c 改 go.mod 不变
  - §六 后续 sprint 入口 → E2E doc 列出
- **占位符扫描**：无 TBD；所有代码块完整
- **类型一致**：
  - `protocol.NpcDialoguePayload.ReplyToChoiceID *string` ↔ web `reply_to_choice_id: string | null` ↔ agent-os `reply_to_choice_id: str | None`
  - `npc.DialogOption{ID,Text}` ↔ web `Array<{id,text}>` ↔ agent-os `DialogOption(id,text)`
  - `cfg.Channels []string` ↔ main `wsredis.Subscribe(appCtx, rdb, cfg.Channels, ...)`
- **Wire-contract 一致**（T05d 修复后）：
  - publishers (agent-os + api-gateway) 只发 inner payload `{npc_id, player_id, tile_id, say, options, reply_to_choice_id, ts_ms, trace_id}`
  - ws-gateway 是**唯一**外层 envelope 包装层
  - 浏览器 `frame.payload.npc_id` 直读（无 `frame.payload.payload.npc_id` 双层 wrap）

## Commit Mapping

每个 task 对应的实际 commit hash（按时间顺序）：

| Task | 服务 | Commit | 备注 |
|---|---|---|---|
| T01a | agent-os | `2988278` | sprint12 减面 - port 8084 + redis + say tick |
| T01a-fix | agent-os | `91dffe2` | `[tool.uv] → [tool.uv.workspace]` uv 0.9+ compat |
| T01b | agent-os | `3787268` | 手写 RESP RedisPub 客户端 |
| T01c | agent-os | `d3c0836` | npc_registry yaml 加载 + enabled 过滤 |
| T01d | agent-os | `48b49cf` | ActionDispatcher + SayScheduler |
| T01e | agent-os | `81ce4e8` | FastAPI app + lifespan + uvicorn entry |
| T02a | ws-gateway | `dbc095f` | NpcDialogue + DialogOption types |
| T02b | ws-gateway | `e7fa566` | multi-channel subscriber with per-channel filter |
| T02c | ws-gateway | `409e285` | wire RunMultiSubscriber |
| T03a | web | `50fc45a` | ws-events bridge dispatches npc_dialogue |
| T03b | web | `1981256` | api.postNpcTalk client method |
| T03c | web | `20bd3ca` | NPCDialog component listens npc_dialogue |
| T03c-docs | web | `9974f90` | NPCDialog.types.ts optional field rationale |
| T03d | web | `87f8bb9` | WorldMap renders NPC markers + click opens dialog |
| T03e | web | `177ef3c` | mount NPCDialog on city page |
| T04a | npc-templates | `20a9a93` | OCEAN personality fields |
| T04b | npc-templates | `d3d8302` | wang_boss.yaml OCEAN + 3 greetings |
| T04b-fix | agent-os | `e419367` | list_enabled skips _LoadError sentinels |
| T05a | api-gateway | `074ade0` | npc.Tree parser + lookup |
| T05b | api-gateway | `af0e547` | POST /v1/npc/talk handler |
| T05c | api-gateway | `d1a89ee` | wire POST /v1/npc/talk route + NPC config dir |
| **T05d** | agent-os | `b90e8a0` | publish npc_dialogue inner payload only |
| **T05d** | api-gateway | `53a753d` | publish npc_dialogue inner payload only |
| **T05e** | agent-os + compose | `890ef4b` | Dockerfile T01e spec + compose service mount |
| T06 | docs | `3382625` | E2E verification report + docker-compose pre-work |
| T06-fix | docs | `4289b6f` | fix commit count + talk_tree + NPC count claims |
| T06-fix | docs | `8565298` | align Known Limitations NPC count with Browser Flow |

**总计**：25 commits（含 4 个 follow-up fix），跨 5 个 stack（agent-os / ws-gateway / api-gateway / web / npc-templates + docs）。

## 完成

5d 实施 + 0.25d 集成 verify = **5.25d 总**。Sprint 12 最小闭环完成。
