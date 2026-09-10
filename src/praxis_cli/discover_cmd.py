"""`praxis executors discover` implementation."""

from __future__ import annotations

from typing import Mapping

from praxis_cli.fields import (
    advertisement_cells,
    authenticated_field,
    installed_field,
    render_cell,
    version_field,
)
from praxis_executors.interface import Executor

# Every column but the executor id, which the block's own header line already
# names rather than repeating as a column.
_PRINTED_COLUMNS = (
    "installed",
    "version",
    "authenticated",
    "auth_transport",
    "capabilities",
)


def build_discover_rows(adapters: Mapping[str, Executor]) -> list[dict]:
    """One row per adapter: the four fields criterion 5 names, plus the
    `auth_transport` that criterion names alongside the capability kinds.

    The advertisement, and the two cells read out of it, come from
    `fields.advertisement_cells` -- the same probe, guards and degraded cells
    `status` takes, so one adapter that could not be asked cannot cost the two
    commands different things. Why the probe comes before the field functions,
    and what a failure leaves behind, is that function's subject.
    """
    rows: list[dict] = []
    for executor_id, executor in adapters.items():
        advertisement, auth_transport, capabilities = advertisement_cells(executor)
        installed = installed_field(executor, advertisement)
        rows.append(
            {
                "executor_id": executor_id,
                "installed": installed,
                "version": version_field(executor),
                "authenticated": authenticated_field(executor, installed),
                "auth_transport": auth_transport,
                "capabilities": capabilities,
            }
        )
    return rows


def print_discover_rows(rows: list[dict]) -> None:
    for row in rows:
        print(f"{row['executor_id']}:")
        for column in _PRINTED_COLUMNS:
            print(f"  {column}: {render_cell(row[column])}".rstrip())


def run_discover(adapters: Mapping[str, Executor]) -> int:
    rows = build_discover_rows(adapters)
    print_discover_rows(rows)
    return 0
