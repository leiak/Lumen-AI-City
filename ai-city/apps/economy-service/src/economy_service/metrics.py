"""Prometheus instrumentation for economy-service."""

from __future__ import annotations

from time import perf_counter

from fastapi import Request, Response
from prometheus_client import (
    CONTENT_TYPE_LATEST,
    Counter,
    Histogram,
    generate_latest,
)
from starlette.middleware.base import BaseHTTPMiddleware, RequestResponseEndpoint
from starlette.responses import Response as StarletteResponse

HTTP_REQUESTS_TOTAL = Counter(
    "economy_http_requests_total",
    "Economy Service HTTP requests.",
    ["route", "method", "status"],
)
HTTP_REQUEST_DURATION = Histogram(
    "economy_http_request_duration_seconds",
    "Economy Service HTTP request duration.",
    ["route", "method"],
)


def metrics_endpoint() -> Response:
    return Response(
        content=generate_latest(),
        media_type=CONTENT_TYPE_LATEST,
    )


class MetricsMiddleware(BaseHTTPMiddleware):
    async def dispatch(
        self,
        request: Request,
        call_next: RequestResponseEndpoint,
    ) -> StarletteResponse:
        started_at = perf_counter()
        response = await call_next(request)
        route = getattr(request.scope.get("route"), "path", "unmatched")
        labels = {
            "route": route,
            "method": request.method,
            "status": str(response.status_code),
        }
        HTTP_REQUESTS_TOTAL.labels(**labels).inc()
        HTTP_REQUEST_DURATION.labels(
            route=route,
            method=request.method,
        ).observe(perf_counter() - started_at)
        return response
