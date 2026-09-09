"""`praxis executors discover` implementation."""

from __future__ import annotations

from typing import Mapping

from praxis_cli.fields import (
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
    "error",
)


def build_discover_rows(adapters: Mapping[str, Executor]) -> list[dict]:
    """One row per adapter, with the same type per column on every row.

    Matches `status_cmd`'s row contract: a failed `.capabilities()` probe
    leaves `auth_transport` empty and `capabilities` an empty list and puts
    the reason in `error`, rather than replacing a column's value with a
    sentence.
    """
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
            row["auth_transport"] = ""
            row["capabilities"] = []
            row["error"] = str(exc)
        else:
            row["auth_transport"] = ",".join(auth_transports(advertisement))
            row["capabilities"] = capability_kinds(advertisement)
            row["error"] = None
        rows.append(row)
    return rows


def print_discover_rows(rows: list[dict]) -> None:
    for row in rows:
        print(f"{row['executor_id']}:")
        for column in _COLUMNS[1:]:
            # `error` is the one column that says nothing on a healthy row.
            if column == "error" and not row[column]:
                continue
            # Spec criterion 5's wording for a row whose probe failed. The row
            # itself keeps an empty list here so a consumer never type-switches;
            # only the human-readable block says `unavailable`.
            if column == "capabilities" and row["error"]:
                print(f"  capabilities: unavailable ({row['error']})")
                continue
            print(f"  {column}: {render_cell(row[column])}".rstrip())


def run_discover(adapters: Mapping[str, Executor]) -> int:
    rows = build_discover_rows(adapters)
    print_discover_rows(rows)
    return 0
