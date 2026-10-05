FROM python:3.12-slim
WORKDIR /app
RUN pip install --no-cache-dir uv
COPY pyproject.toml .
RUN uv pip install --system --no-cache .
COPY src/ ./src/
ENV PYTHONPATH=/app/src
EXPOSE 9200
CMD ["uvicorn", "memory_service.main:app", "--host", "0.0.0.0", "--port", "9200"]
