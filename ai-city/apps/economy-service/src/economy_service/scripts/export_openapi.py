from __future__ import annotations

import argparse
import json
from pathlib import Path
from typing import Any

from economy_service.app import app


def build_openapi_document() -> dict[str, Any]:
    return app.openapi()


def write_openapi_document(path: Path) -> Path:
    path.write_text(
        json.dumps(build_openapi_document(), ensure_ascii=False, indent=2, sort_keys=True) + "\n",
        encoding="utf-8",
    )
    return path


def main() -> None:
    parser = argparse.ArgumentParser(description="Export the economy-service OpenAPI contract")
    parser.add_argument(
        "output",
        nargs="?",
        default=Path(__file__).parents[4] / "openapi.json",
        type=Path,
    )
    args = parser.parse_args()
    output = write_openapi_document(args.output)
    print(output)


if __name__ == "__main__":
    main()
