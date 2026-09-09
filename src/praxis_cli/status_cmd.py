"""Bare `praxis executors` status table and `--json` implementation."""

from __future__ import annotations

import json
import logging
from typing import Mapping

import jsonschema

from praxis_cli.fields import (
    PROBE_FAILED,
    UNAVAILABLE,
    UNDETERMINED,
    auth_transports,
    capability_kinds,
    note_probe_failure,
    render_cell,
    status_field,
)
from praxis_executors.interface import Executor, ExecutorAvailability

# Spec criterion 6 names exactly these four, for both the table and `--json`.
_COLUMNS = ("executor_id", "auth_transport", "status", "capabilities")

# The `--json` row shape, machine-readable rather than only described in prose.
#
# Criterion 6 fixes the row at those four fields, so a row whose probe failed
# states its reason inside them rather than in a fifth key. Two of the four
# therefore carry more than one kind of value, and a consumer that assumed
# `capabilities` were always a list would iterate the reason string one
# character at a time. Both unions are declared here so that shape is something
# a consumer reads and validates against, not something it discovers at
# runtime -- and so neither can widen again without this schema saying so.
STATUS_ROW_SCHEMA = {
    "$schema": "https://json-schema.org/draft/2020-12/schema",
    "title": "praxis executors --json row",
    "type": "object",
    "required": list(_COLUMNS),
    "additionalProperties": False,
    "properties": {
        "executor_id": {
            "type": "string",
            "description": "The id the adapter is registered under, always present.",
        },
        "auth_transport": {
            "type": "string",
            "description": (
                f"Every transport the advertisement names, joined on ',', or the "
                f"bare string '{UNAVAILABLE}' when the advertisement could not be "
                f"read. Empty means the advertisement named no transport at all."
            ),
        },
        "status": {
            "type": "string",
            "description": (
                "The adapter's own availability verdict, or "
                f"'{UNDETERMINED}' when the health probe itself raised."
            ),
            "enum": [
                *(availability.value for availability in ExecutorAvailability),
                UNDETERMINED,
            ],
        },
        "capabilities": {
            "description": (
                "A list of capability kinds when the adapter answered, or the "
                f"string '{UNAVAILABLE} (<reason>)' when it could not be asked. "
                "Check the type before iterating."
            ),
            "oneOf": [
                {"type": "array", "items": {"type": "string"}},
                {"type": "string", "pattern": f"^{UNAVAILABLE} \\("},
            ],
        },
    },
}

_LOGGER = logging.getLogger(__name__)

# Built once: `print_status_json` validates every row it emits against the
# schema it publishes, so the two cannot drift apart unnoticed.
_ROW_VALIDATOR = jsonschema.Draft202012Validator(STATUS_ROW_SCHEMA)


def build_status_rows(adapters: Mapping[str, Executor]) -> list[dict]:
    """One row per adapter, carrying exactly the four columns criterion 6 names.

    A failed `.capabilities()` probe puts its reason in the `capabilities`
    cell, in criterion 5's own wording (`unavailable (<reason>)`), and marks
    `auth_transport` unavailable rather than empty -- empty is what a
    conforming advertisement that names no transport already means.

    Catches `fields.PROBE_FAILED` rather than `ExecutorError` alone: an
    adapter's transport layer can surface a malformed response that is not its
    own `ExecutorError`, and one such adapter must degrade its own row rather
    than take the whole command down. Which failures those are is `fields`'
    subject, spelled once for `discover` and `match` too.

    The advertisement is read inside that guard rather than after it: an adapter
    that answers with an advertisement missing a key its schema requires has
    failed this probe just as much as one that raised, and one such adapter must
    cost its own row and not the table.

    The advertisement is probed first and then handed to `status_field`, so an
    adapter whose `.capabilities()` and `.health()` hit the same endpoint is
    asked once rather than waited on twice at its own timeout -- the same order
    `build_discover_rows` takes for the same reason. Which adapters that covers
    is `fields`' subject, not this module's.
    """
    rows: list[dict] = []
    for executor_id, executor in adapters.items():
        try:
            advertisement = executor.capabilities()
            # Comma without a space: `auth_transport` is not the last column,
            # and a reader scanning the table down a column should not have to
            # guess where one cell's value ends.
            auth_transport = ",".join(auth_transports(advertisement))
            capabilities = capability_kinds(advertisement)
        except PROBE_FAILED as exc:
            # A failed advertisement probe is not a verdict about availability,
            # so `status_field` still asks `health()` below: a row that names
            # its reason beats a row that only says the status is unknown.
            note_probe_failure(executor, "capabilities", exc)
            advertisement = None
            auth_transport = UNAVAILABLE
            capabilities = f"{UNAVAILABLE} ({exc})"
        rows.append(
            {
                "executor_id": executor_id,
                "auth_transport": auth_transport,
                "status": status_field(executor, advertisement),
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
    """Prints the rows as one JSON line, reporting any that `STATUS_ROW_SCHEMA`
    rejects.

    The schema is the contract a `--json` consumer reads, so the command that
    emits the rows is what checks itself against it: a row shape that widens
    without the schema saying so is reported here, once, instead of reaching a
    consumer that validates against a schema no longer describing what it got.

    Reported rather than raised on, and the row still prints: every failure in
    this module degrades what it can and emits the rest, and a row that breaks
    its own contract is no reason to withhold the other adapters' rows.
    """
    for row in rows:
        for error in _ROW_VALIDATOR.iter_errors(row):
            _LOGGER.warning(
                "row for %s does not match STATUS_ROW_SCHEMA: %s",
                row.get("executor_id", "<unnamed>"),
                error.message,
            )
    print(json.dumps(rows))


def run_status(adapters: Mapping[str, Executor], *, as_json: bool) -> int:
    rows = build_status_rows(adapters)
    if as_json:
        print_status_json(rows)
    else:
        print_status_table(rows)
    return 0
