from __future__ import annotations

import importlib.resources
from pathlib import Path

SCHEMA_DIR: Path = Path(importlib.resources.files("praxis_contracts") / "schemas" / "v1")


def schema_path(filename: str) -> Path:
    return SCHEMA_DIR / filename
