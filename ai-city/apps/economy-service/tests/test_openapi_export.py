from __future__ import annotations

import json
from pathlib import Path

from economy_service.scripts.export_openapi import write_openapi_document


def test_openapi_export_contains_service_contract(tmp_path: Path) -> None:
    output = write_openapi_document(tmp_path / "openapi.json")

    assert output == tmp_path / "openapi.json"
    document = json.loads(output.read_text(encoding="utf-8"))
    assert document["info"]["title"] == "AI City - Economy Service"
    for path in (
        "/health",
        "/api/v1/wallet/{user_id}",
        "/api/v1/wallet/transfer",
        "/api/v1/wallet/purchase",
        "/api/v1/admin/central-bank/emit",
        "/api/v1/admin/central-bank/sink",
        "/v1/marketplace/npc-templates",
        "/v1/marketplace/purchase",
    ):
        assert path in document["paths"]
