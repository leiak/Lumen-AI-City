FROM python:3.12-slim

WORKDIR /app

# 安装 uv
RUN pip install --no-cache-dir uv

# 复制项目文件
COPY pyproject.toml ./
COPY src/ ./src/

# 安装依赖
RUN uv pip install --system --no-cache .

EXPOSE 8000

CMD ["uvicorn", "agent_os.app:app", "--host", "0.0.0.0", "--port", "8000"]
