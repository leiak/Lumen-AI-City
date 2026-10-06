# ============================================
# economy-service (Python 3.12 + FastAPI) — 构建上下文 = ai-city/
# ============================================
# 与 apps/agent-os/Dockerfile 同款上下文（repo 根）；compose 里
#   build: { context: ., dockerfile: apps/economy-service/Dockerfile }
# 强制走根 context，所以这里 COPY 路径必须以 apps/economy-service/ 起头。

FROM python:3.12-slim

# 走清华源，与 agent-os 对齐（files.pythonhosted.org 在本环境偶发 read timeout）
ENV PIP_INDEX_URL=https://pypi.tuna.tsinghua.edu.cn/simple
ENV UV_INDEX_URL=https://pypi.tuna.tsinghua.edu.cn/simple

# uv 安装到 system python（避免容器内 venv 路径漂移）
# wget 给 compose healthcheck 用 ;python:3.12-slim 默认不带。
RUN apt-get update \
 && apt-get install -y --no-install-recommends wget ca-certificates \
 && rm -rf /var/lib/apt/lists/*
RUN pip install --no-cache-dir uv

WORKDIR /app

# 镜像内只放 economy-service 自己的代码 + pyproject
COPY apps/economy-service/pyproject.toml ./pyproject.toml
COPY apps/economy-service/src ./src
# scripts/ 留 acceptance_economy_v1.py；容器内通过
#   docker compose exec economy-service python scripts/acceptance_economy_v1.py 调
COPY apps/economy-service/scripts ./scripts

# 镜像内安装 agent-os 仅供 acceptance_economy_v1.py step 10 调
# agent_os.bt.actions.npc_sell_to_player + agent_os.bt.state.BTState。装无依赖版
#（--no-deps）避开 litellm/asyncpg 等 agent-os 全栈依赖；脚本只 import bt 子树。
# httpx 已通过 economy-service 自家 pyproject 引入。pydantic-settings 是
# agent_os.config 的硬依赖（agent_os/__init__.py:7 import 触发），单独装一份。
COPY apps/agent-os/pyproject.toml /tmp/agent-os/pyproject.toml
COPY apps/agent-os/src /tmp/agent-os/src
RUN uv pip install --system --no-cache --no-deps /tmp/agent-os \
 && uv pip install --system --no-cache pydantic-settings

# Acceptance binary sanity check: py_compile 在 build 时校验主入口语法
RUN python -m py_compile src/economy_service/app.py

# 把 src 加到 PYTHONPATH，让 `economy_service.*` import 可解析
ENV PYTHONPATH=/app/src

# system install —— 与 agent-os 同源；pyproject.toml 的 dependencies
# 覆盖 aiokafka/fastapi/uvicorn/asyncpg/sqlalchemy/redis/httpx/pydantic
RUN uv pip install --system --no-cache .

# 默认环境变量（compose 会覆盖 DATABASE_URL/REDIS_URL/KAFKA_BOOTSTRAP_SERVERS/
# ADMIN_TOKEN/EMIT_INTERVAL_HOURS）
ENV REDIS_URL=redis://redis:6379/0
ENV KAFKA_BOOTSTRAP_SERVERS=kafka:9092
ENV ADMIN_TOKEN=dev-admin-token
ENV EMIT_INTERVAL_HOURS=6
ENV PYTHONUNBUFFERED=1

EXPOSE 8005

# 与 agent-os 同模式：system python + uvicorn 启动 FastAPI app。
CMD ["uvicorn", "economy_service.app:app", "--host", "0.0.0.0", "--port", "8005"]