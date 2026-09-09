from __future__ import annotations

import importlib.metadata


def main() -> None:
    print(importlib.metadata.version("praxis-contracts"))
