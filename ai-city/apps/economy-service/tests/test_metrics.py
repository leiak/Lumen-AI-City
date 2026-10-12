from fastapi import FastAPI
from fastapi.testclient import TestClient

from economy_service.metrics import MetricsMiddleware, metrics_endpoint


def build_metrics_app() -> FastAPI:
    app = FastAPI()
    app.add_middleware(MetricsMiddleware)

    @app.get("/wallet/test")
    async def test_route():
        return {"ok": True}

    @app.get("/metrics", include_in_schema=False)
    async def metrics():
        return metrics_endpoint()

    return app


def test_metrics_exposes_route_level_counters():
    with TestClient(build_metrics_app()) as client:
        assert client.get("/wallet/test").status_code == 200
        payload = client.get("/metrics").text

    assert 'route="/wallet/test"' in payload
    assert 'method="GET"' in payload
    assert 'status="200"' in payload
    assert "economy_http_request_duration_seconds" in payload
