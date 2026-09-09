"""`praxis executors discover` implementation."""

from __future__ import annotations

from typing import Mapping

from praxis_cli.fields import (
    auth_transports,
    authenticated_field,
    capability_kinds,
    installed_field,
    version_field,
)
from praxis_executors.interface import Executor, ExecutorError

_COLUMNS = (
    "executor_id",
    "installed",
    "version",
    "authenticated",
    "auth_transport",
    "capabilities",
)


def build_discover_rows(adapters: Mapping[str, Executor]) -> list[dict]:
    rows: list[dict] = []
    for executor_id, executor in adapters.items():
        installed = installed_field(executor)
        row = {
            "executor_id": executor_id,
            "installed": installed,
            "version": version_field(executor),
            "authenticated": authenticated_field(executor, installed),
        }
        try:
            advertisement = executor.capabilities()
        except ExecutorError as exc:
            # One adapter whose backing CLI or service is absent must not take
            # the whole report down -- its row degrades, the rest still print.
            row["auth_transport"] = "unavailable"
            row["capabilities"] = f"unavailable ({exc})"
        else:
            row["auth_transport"] = ", ".join(auth_transports(advertisement))
            row["capabilities"] = capability_kinds(advertisement)
        rows.append(row)
    return rows


def print_discover_rows(rows: list[dict]) -> None:
    for row in rows:
        print(f"{row['executor_id']}:")
        for column in _COLUMNS[1:]:
            print(f"  {column}: {row[column]}")


def run_discover(adapters: Mapping[str, Executor]) -> int:
    rows = build_discover_rows(adapters)
    print_discover_rows(rows)
    return 0
