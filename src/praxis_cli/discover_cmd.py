"""`praxis executors discover` implementation."""

from __future__ import annotations

from typing import Mapping

from praxis_cli.fields import (
    UNAVAILABLE,
    auth_transports,
    authenticated_field,
    capability_kinds,
    installed_field,
    render_cell,
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

# Every column but the id, which the block's own header line already names.
_PRINTED_COLUMNS = _COLUMNS[1:]


def build_discover_rows(adapters: Mapping[str, Executor]) -> list[dict]:
    """One row per adapter: the four fields criterion 5 names, plus the
    `auth_transport` that criterion names alongside the capability kinds.

    Matches `status_cmd`'s row contract: a failed `.capabilities()` probe
    states its reason in the `capabilities` cell, in criterion 5's own wording
    (`unavailable (<reason>)`), and marks `auth_transport` unavailable rather
    than empty -- empty is what a conforming advertisement that names no
    transport already means.

    The advertisement is probed first and then handed to `installed_field`, so
    an adapter whose `.capabilities()` and `.health()` hit the same endpoint
    is asked once rather than waited on twice at its own timeout.
    """
    rows: list[dict] = []
    for executor_id, executor in adapters.items():
        try:
            advertisement = executor.capabilities()
        except (ExecutorError, ValueError) as exc:
            # One adapter whose backing CLI or service is absent must not take
            # the whole report down -- its row degrades, the rest still print.
            advertisement = None
            auth_transport = UNAVAILABLE
            capabilities = f"{UNAVAILABLE} ({exc})"
        else:
            auth_transport = ",".join(auth_transports(advertisement))
            capabilities = capability_kinds(advertisement)
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
