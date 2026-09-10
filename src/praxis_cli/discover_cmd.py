"""`praxis executors discover` implementation."""

from __future__ import annotations

from typing import Mapping

from praxis_cli.fields import (
    PROBE_FAILED,
    MalformedAdvertisement,
    auth_transports,
    authenticated_field,
    capability_kinds,
    installed_field,
    render_cell,
    unavailable_cells,
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

    A failed `.capabilities()` probe degrades this row to the cells
    `fields.unavailable_cells` words, the row contract `status` shares.

    The advertisement is probed first and then handed to `installed_field`; why
    that ordering saves a round trip, and why a failed probe still costs one,
    is `praxis_cli.fields`' subject, where the substitution predicate lives.

    A failed probe is caught on `fields.PROBE_FAILED`, the one set `status` and
    `match` also degrade a row on, so the three commands cannot disagree about
    which failure is survivable. Reading the advertisement afterwards is
    guarded separately, on `fields.MalformedAdvertisement`, which is the class
    that says why the two are worth keeping apart.
    """
    rows: list[dict] = []
    for executor_id, executor in adapters.items():
        try:
            advertisement = executor.capabilities()
        except PROBE_FAILED as exc:
            # One adapter whose backing CLI or service is absent must not take
            # the whole report down -- its row degrades, the rest still print.
            advertisement = None
            auth_transport, capabilities = unavailable_cells(executor, "capabilities", exc)
        else:
            try:
                auth_transport = ",".join(auth_transports(advertisement))
                capabilities = capability_kinds(advertisement)
            except MalformedAdvertisement as exc:
                # The advertisement is dropped with the cells: it answered, but
                # not conformingly, so it is no evidence about the backing
                # service and `installed_field` still has to ask `health()`.
                advertisement = None
                auth_transport, capabilities = unavailable_cells(executor, "capabilities", exc)
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
