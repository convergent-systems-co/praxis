"""Bare `praxis executors` status table and `--json` implementation."""

from __future__ import annotations

import json
from typing import Mapping

from praxis_cli.fields import auth_transports, capability_kinds, render_cell
from praxis_executors.interface import Executor, ExecutorError

_COLUMNS = ("executor_id", "auth_transport", "status", "capabilities", "error")


def build_status_rows(adapters: Mapping[str, Executor]) -> list[dict]:
    """One row per adapter, with the same type per column on every row.

    A failed `.capabilities()` probe leaves `auth_transport` empty and
    `capabilities` an empty list, and puts the reason in `error`, so a
    machine consumer of `--json` never has to type-switch per row.
    """
    rows: list[dict] = []
    for executor_id, executor in adapters.items():
        status = executor.health().value
        try:
            advertisement = executor.capabilities()
        except ExecutorError as exc:
            rows.append(
                {
                    "executor_id": executor_id,
                    "auth_transport": "",
                    "status": status,
                    "capabilities": [],
                    "error": str(exc),
                }
            )
            continue
        rows.append(
            {
                "executor_id": executor_id,
                # Comma without a space: the table below splits on whitespace
                # runs, so no cell may contain one unless it is the last.
                "auth_transport": ",".join(auth_transports(advertisement)),
                "status": status,
                "capabilities": capability_kinds(advertisement),
                "error": None,
            }
        )
    return rows


def print_status_table(rows: list[dict]) -> None:
    columns = list(_COLUMNS)
    # `error` is free text and the only cell that can contain a space, so it
    # stays last -- and disappears entirely when every probe succeeded.
    if not any(row.get("error") for row in rows):
        columns.remove("error")

    lines = [[column.upper() for column in columns]]
    lines.extend([render_cell(row.get(column)) for column in columns] for row in rows)
    widths = [max(len(line[i]) for line in lines) for i in range(len(columns))]

    for line in lines:
        print("  ".join(cell.ljust(width) for cell, width in zip(line, widths)).rstrip())


def print_status_json(rows: list[dict]) -> None:
    print(json.dumps(rows))


def run_status(adapters: Mapping[str, Executor], *, as_json: bool) -> int:
    rows = build_status_rows(adapters)
    if as_json:
        print_status_json(rows)
    else:
        print_status_table(rows)
    return 0
