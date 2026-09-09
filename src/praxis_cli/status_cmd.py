"""Bare `praxis executors` status table and `--json` implementation."""

from __future__ import annotations

import json

from praxis_cli.fields import auth_transports, capability_kinds, fallback_executor_id
from praxis_executors.interface import Executor, ExecutorError

_COLUMNS = ("executor_id", "auth_transport", "status", "capabilities")


def build_status_rows(adapters: list[Executor]) -> list[dict]:
    rows: list[dict] = []
    for index, executor in enumerate(adapters):
        status = executor.health().value
        try:
            advertisement = executor.capabilities()
        except ExecutorError as exc:
            rows.append(
                {
                    "executor_id": fallback_executor_id(executor, index),
                    "auth_transport": f"unavailable ({exc})",
                    "status": status,
                    "capabilities": f"unavailable ({exc})",
                }
            )
            continue
        rows.append(
            {
                "executor_id": advertisement["executor_id"],
                "auth_transport": ", ".join(auth_transports(advertisement)),
                "status": status,
                "capabilities": capability_kinds(advertisement),
            }
        )
    return rows


def print_status_table(rows: list[dict]) -> None:
    for row in rows:
        print(" ".join(str(row[column]) for column in _COLUMNS))


def print_status_json(rows: list[dict]) -> None:
    print(json.dumps(rows))


def run_status(adapters: list[Executor], *, as_json: bool) -> int:
    rows = build_status_rows(adapters)
    if as_json:
        print_status_json(rows)
    else:
        print_status_table(rows)
    return 0
