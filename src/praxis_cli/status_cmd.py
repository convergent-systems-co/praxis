"""Bare `praxis executors` status table and `--json` implementation."""

from __future__ import annotations

import json
import logging
from typing import Mapping

import jsonschema

from praxis_cli.fields import (
    UNAVAILABLE,
    UNDETERMINED,
    advertisement_cells,
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

    The advertisement, and the two cells read out of it, come from
    `fields.advertisement_cells` -- the same probe, guards and degraded cells
    `discover` takes, so an adapter that costs one command a line cannot cost
    the other the whole command. What a failed probe leaves behind, and why
    `status_field` is still asked for a row that failed one, is that function's
    subject.
    """
    rows: list[dict] = []
    for executor_id, executor in adapters.items():
        advertisement, auth_transport, capabilities = advertisement_cells(executor)
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
