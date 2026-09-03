FROM python:3.12-slim
WORKDIR /app
RUN pip install uv
COPY pyproject.toml ./
COPY src/ ./src/
RUN uv pip install --system .
EXPOSE 8004
CMD ["uvicorn", "bt_editor_api.app:app", "--host", "0.0.0.0", "--port", "8004"]
