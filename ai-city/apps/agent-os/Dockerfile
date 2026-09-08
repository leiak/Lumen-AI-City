# ============================================
# agent-os (Python 3.12 + FastAPI) — 构建上下文 = ai-city/
# ============================================
# 构建上下文是 repo 根：apps/agent-os 下面没有 pyproject.toml 之外的
# 锁文件，uv 直接读 pyproject.toml 的 dependencies 即可。templates 由
# compose 的 bind mount 注入；镜像里只放一个空占位目录避免 ImageBuild 失败。

FROM python:3.12-slim

# uv 安装到 system python（与 dev 环境一致，避免 venv 路径漂移）
RUN pip install --no-cache-dir uv

WORKDIR /app

# 仅复制 agent-os 自己的代码 + pyproject.toml
COPY apps/agent-os/pyproject.toml ./pyproject.toml
COPY apps/agent-os/src ./src

# 把 src 加到 PYTHONPATH，让 `agent_os.*` import 可解析
ENV PYTHONPATH=/app/src

# system install —— 容器里没有 dev venv，system 路径更直观
RUN uv pip install --system --no-cache .

# 镜像内占位目录；容器跑起来时由 compose bind mount 覆盖（:ro）。
# 即便没 mount 进来，app factory 的 LoadAll 也会 best-effort 跳过。
RUN mkdir -p /etc/aicity/npc-templates

# 默认环境变量（compose 会覆盖 REDIS_URL / NPC_TEMPLATES_DIR / HTTP_PORT）
ENV REDIS_URL=redis://redis:6379/0
ENV REDIS_CHANNEL_NPC_DIALOGUE=aicity:npc_dialogue
ENV NPC_TEMPLATES_DIR=/etc/aicity/npc-templates
ENV HTTP_PORT=8084
ENV LOG_LEVEL=info
ENV SERVICE_NAME=agent-os
ENV SAY_TICK_SECONDS=5.0
# PYTHONUNBUFFERED 保证 compose logs 能立即看到 startup banner
ENV PYTHONUNBUFFERED=1

EXPOSE 8084

# 直接走 system python + main.py 的 uvicorn factory 启动。
# 已通过 `uv pip install --system` 把 fastapi/uvicorn/pydantic 等
# 装进 system site-packages；pyproject.toml 里注册的 `agent-os`
# script 也指向同一个 entry。两路径同源，避免 compose vs local 漂移。
CMD ["python", "-m", "agent_os.main"]