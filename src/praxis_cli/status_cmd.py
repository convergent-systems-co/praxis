"""Bare `praxis executors` status table and `--json` implementation."""

from __future__ import annotations

import json
from typing import Mapping

from praxis_cli.fields import UNAVAILABLE, auth_transports, capability_kinds, render_cell
from praxis_executors.interface import Executor, ExecutorError

# Spec criterion 6 names exactly these four, for both the table and `--json`.
_COLUMNS = ("executor_id", "auth_transport", "status", "capabilities")

# A `.health()` call that raised returned no verdict at all, so no
# `ExecutorAvailability` value describes it. Reporting one of the three real
# ones (`degraded`, say) would claim a probe result that never happened, and a
# `--json` consumer filtering on it could not tell the two apart.
_UNDETERMINED_STATUS = "unknown"


def build_status_rows(adapters: Mapping[str, Executor]) -> list[dict]:
    """One row per adapter, carrying exactly the four columns criterion 6 names.

    A failed `.capabilities()` probe puts its reason in the `capabilities`
    cell, in criterion 5's own wording (`unavailable (<reason>)`), and marks
    `auth_transport` unavailable rather than empty -- empty is what a
    conforming advertisement that names no transport already means.

    Catches `ValueError` alongside `ExecutorError`: an adapter's transport
    layer can surface a malformed response (e.g. `json.JSONDecodeError`,
    itself a `ValueError` subclass) that is not its own `ExecutorError`, and
    one such adapter must degrade its own row rather than take the whole
    command down.
    """
    rows: list[dict] = []
    for executor_id, executor in adapters.items():
        try:
            status = executor.health().value
        except (ExecutorError, ValueError):
            status = _UNDETERMINED_STATUS
        try:
            advertisement = executor.capabilities()
        except (ExecutorError, ValueError) as exc:
            # The advertisement probe is asked independently of `health()`:
            # one failing says nothing about the other, and a row that names
            # its reason beats a row that only says the status is unknown.
            auth_transport = UNAVAILABLE
            capabilities = f"{UNAVAILABLE} ({exc})"
        else:
            # Comma without a space: `auth_transport` is not the last column,
            # and a reader scanning the table down a column should not have to
            # guess where one cell's value ends.
            auth_transport = ",".join(auth_transports(advertisement))
            capabilities = capability_kinds(advertisement)
        rows.append(
            {
                "executor_id": executor_id,
                "auth_transport": auth_transport,
                "status": status,
                "capabilities": capabilities,
            }
        )
    return rows


def print_status_table(rows: list[dict]) -> None:
    columns = list(_COLUMNS)
    lines = [[column.upper() for column in columns]]
    lines.extend([render_cell(row[column]) for column in columns] for row in rows)
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
