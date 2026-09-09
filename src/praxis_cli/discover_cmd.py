"""`praxis executors discover` implementation."""

from __future__ import annotations

from praxis_cli.fields import authenticated_field, capability_kinds, installed_field, version_field
from praxis_executors.interface import Executor, ExecutorError

_COLUMNS = ("executor_id", "installed", "version", "authenticated", "capabilities")


def build_discover_rows(adapters: list[Executor]) -> list[dict]:
    rows: list[dict] = []
    for executor in adapters:
        installed = installed_field(executor)
        row = {
            "executor_id": type(executor).__name__,
            "installed": installed,
            "version": version_field(executor),
            "authenticated": authenticated_field(executor, installed),
        }
        try:
            advertisement = executor.capabilities()
        except ExecutorError as exc:
            row["capabilities"] = f"unavailable ({exc})"
            rows.append(row)
            continue
        row["executor_id"] = advertisement["executor_id"]
        row["capabilities"] = capability_kinds(advertisement)
        rows.append(row)
    return rows


def print_discover_rows(rows: list[dict]) -> None:
    for row in rows:
        print(f"{row['executor_id']}:")
        for column in _COLUMNS[1:]:
            print(f"  {column}: {row[column]}")


def run_discover(adapters: list[Executor]) -> int:
    rows = build_discover_rows(adapters)
    print_discover_rows(rows)
    return 0
