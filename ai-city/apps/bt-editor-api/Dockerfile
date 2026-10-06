# ============================================
# bt-editor-api (Python 3.12 + FastAPI) — 构建上下文 = ai-city/
# ============================================
# Phase C.2: workspace dependency on apps/agent-os (C.1 GA BT runtime).
# 构建上下文是 repo 根：uv 同时拉 bt-editor-api + agent-os 两个 workspace
# member。镜像里只跑 bt-editor-api，agent-os 作为 importable lib 提供
# `agent_os.bt.{load_tree, evaluator.tick, ...}`.

FROM python:3.12-slim

# 网络修正（与 agent-os Dockerfile 对齐）
ENV PIP_INDEX_URL=https://pypi.tuna.tsinghua.edu.cn/simple
ENV UV_INDEX_URL=https://pypi.tuna.tsinghua.edu.cn/simple

RUN pip install --no-cache-dir uv

WORKDIR /app

# 同时复制 workspace 锁文件 + 两个相关 member。
# apps/agent-os: C.1 BT runtime（load_tree / evaluator.tick / 校验 schema）。
# apps/bt-editor-api: 本服务。
COPY pyproject.toml uv.lock ./
COPY apps/bt-editor-api ./apps/bt-editor-api
COPY apps/agent-os ./apps/agent-os

# 把两个 src 都加到 PYTHONPATH，让 bt-editor-api 既能 import 自己
# 也能 import agent_os.bt.*；与 dev 环境（uv workspace）行为一致。
ENV PYTHONPATH=/app/apps/bt-editor-api/src:/app/apps/agent-os/src

# workspace install：只装 bt-editor-api 的运行时 deps（拉它的 transitive
# deps 时会自动满足 agent-os 的 deps，但不把 agent-os 的 console_scripts
# 等非必要部分强加进镜像）。--no-dev 跳过 pytest/mypy 等。
RUN uv pip install --system --no-cache \
    ./apps/agent-os \
    ./apps/bt-editor-api

EXPOSE 8004

CMD ["uvicorn", "bt_editor_api.app:app", "--host", "0.0.0.0", "--port", "8004"]
