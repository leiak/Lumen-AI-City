FROM python:3.12-slim
WORKDIR /app
RUN pip install uv
COPY pyproject.toml ./
COPY src/ ./src/
RUN uv pip install --system .
EXPOSE 8003
CMD ["uvicorn", "notification_engine.app:app", "--host", "0.0.0.0", "--port", "8003"]
