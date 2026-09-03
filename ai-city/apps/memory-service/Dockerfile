FROM python:3.12-slim
WORKDIR /app
RUN pip install uv
COPY pyproject.toml ./
COPY src/ ./src/
RUN uv pip install --system .
EXPOSE 8001
CMD ["uvicorn", "memory_service.app:app", "--host", "0.0.0.0", "--port", "8001"]
